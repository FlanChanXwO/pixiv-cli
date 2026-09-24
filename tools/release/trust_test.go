package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type closeErrorWriter struct {
	strings.Builder
}

func (writer *closeErrorWriter) Close() error {
	return errors.New("close github output")
}

func TestReleaseTrustSourceUsesImmutableTagOnDefaultBranch(t *testing.T) {
	// hook runner 会导出 GIT_DIR/GIT_WORK_TREE/GIT_INDEX_FILE，而 git 让它们
	// 优先于 `-C`；fixture 命令必须显式钉到临时仓库，否则会作用于开发者真实仓库。
	t.Setenv("PATH", "/usr/bin:/bin"+string(os.PathListSeparator)+os.Getenv("PATH"))
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", repo}, args...)...)
		command.Env = append(os.Environ(),
			"GIT_DIR="+filepath.Join(repo, ".git"),
			"GIT_WORK_TREE="+repo,
			"GIT_INDEX_FILE="+filepath.Join(repo, ".git", "index"),
		)
		body, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, body)
		}
		return strings.TrimSpace(string(body))
	}
	git("init", "-b", "main")
	git("config", "user.name", "release-test")
	git("config", "user.email", "release-test@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "README.md")
	git("commit", "-m", "fixture")
	git("tag", "v1.2.3")
	commit := git("rev-parse", "HEAD")
	git("update-ref", "refs/remotes/origin/main", commit)

	trust, err := verifySourceTrust(repo, "v1.2.3", "main")
	if err != nil {
		t.Fatalf("verify source trust: %v", err)
	}
	if trust.Commit != commit || trust.Version != "1.2.3" {
		t.Fatalf("source trust = %#v, want commit %s version 1.2.3", trust, commit)
	}

	git("checkout", "--orphan", "other")
	git("rm", "-rf", ".")
	if err := os.WriteFile(filepath.Join(repo, "other.txt"), []byte("other\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "other.txt")
	git("commit", "-m", "other")
	git("tag", "v1.2.4")
	if _, err := verifySourceTrust(repo, "v1.2.4", "main"); err == nil {
		t.Fatal("source trust accepted tag outside the default-branch ancestry")
	}
}

func TestReleaseTrustPublishedReleaseAndHandoff(t *testing.T) {
	t.Parallel()

	commit := strings.Repeat("a", 40)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(writer, "missing auth", http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/repos/FlanChanXwO/pixiv-cli/releases/tags/v1.2.3":
			fmt.Fprint(writer, `{"tag_name":"v1.2.3","draft":false,"prerelease":false,"published_at":"2026-09-20T00:00:00Z"}`)
		case "/repos/FlanChanXwO/pixiv-cli/actions/runs/42":
			fmt.Fprintf(writer, `{"name":"Release","conclusion":"success","head_sha":%q}`, commit)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	client := githubReleaseClient{BaseURL: server.URL, Token: "test-token", HTTP: server.Client()}
	if err := verifyPublishedRelease(client, "FlanChanXwO/pixiv-cli", "v1.2.3", true); err != nil {
		t.Fatalf("verify published release: %v", err)
	}
	if err := verifyReleaseHandoff(client, "FlanChanXwO/pixiv-cli", 42, "Release", commit); err != nil {
		t.Fatalf("verify release handoff: %v", err)
	}
	if err := verifyReleaseHandoff(client, "FlanChanXwO/pixiv-cli", 42, "Release", ""); err != nil {
		t.Fatalf("verify recovery handoff without head binding: %v", err)
	}
}

func TestVerifyPublishedReleaseStableRejectsPrereleaseTagEvenWhenReleaseFlagIsFalse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/repos/FlanChanXwO/pixiv-cli/releases/tags/v1.2.3-rc.1" {
			http.NotFound(writer, request)
			return
		}
		fmt.Fprint(writer, `{"tag_name":"v1.2.3-rc.1","draft":false,"prerelease":false,"published_at":"2026-09-20T00:00:00Z"}`)
	}))
	t.Cleanup(server.Close)

	client := githubReleaseClient{BaseURL: server.URL, Token: "test-token", HTTP: server.Client()}
	if err := verifyPublishedRelease(client, "FlanChanXwO/pixiv-cli", "v1.2.3-rc.1", true); err == nil {
		t.Fatal("stable published Release verification accepted a semantic prerelease tag")
	}
}

func TestAppendGitHubOutputReturnsCloseError(t *testing.T) {
	t.Parallel()

	writer := &closeErrorWriter{}
	err := appendGitHubOutputTo(writer, map[string]string{"commit": strings.Repeat("a", 40)})
	if err == nil || !strings.Contains(err.Error(), "close github output") {
		t.Fatalf("appendGitHubOutputTo close error = %v, want close failure", err)
	}
}

// TestReleaseTrustGitHelpersIgnoreInheritedGitEnv 是仓库破坏性 bug 的回归测试：
// 继承的 GIT_DIR 会覆盖 `git -C`，而 pre-commit 运行 hook 时会导出
// GIT_DIR/GIT_WORK_TREE/GIT_INDEX_FILE。若不隔离，release 工具的 git 调用会
// 作用在开发者的真实仓库上，把 fixture 的 tag/身份写进去。
func TestReleaseTrustGitHelpersIgnoreInheritedGitEnv(t *testing.T) {
	fixture := t.TempDir()
	if err := runGit(fixture, "init", "-b", "main"); err != nil {
		t.Fatalf("init fixture: %v", err)
	}
	if err := runGit(fixture, "config", "user.name", "fixture"); err != nil {
		t.Fatalf("configure name: %v", err)
	}
	if err := runGit(fixture, "config", "user.email", "fixture@example.invalid"); err != nil {
		t.Fatalf("configure email: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runGit(fixture, "add", "README.md"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := runGit(fixture, "commit", "-m", "fixture"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	decoy := t.TempDir()
	if err := runGit(decoy, "init", "-b", "main"); err != nil {
		t.Fatalf("init decoy: %v", err)
	}
	t.Setenv("GIT_DIR", filepath.Join(decoy, ".git"))
	t.Setenv("GIT_WORK_TREE", decoy)
	if err := runGit(fixture, "tag", "v1.2.3"); err != nil {
		t.Fatalf("tag fixture: %v", err)
	}

	toplevel, err := captureGit(fixture, "rev-parse", "--show-toplevel")
	if err != nil {
		t.Fatalf("capture toplevel: %v", err)
	}
	if !samePath(toplevel, fixture) {
		t.Fatalf("git helpers resolved %q, want the fixture root %q", toplevel, fixture)
	}
	if _, err := captureGit(decoy, "rev-parse", "--verify", "refs/tags/v1.2.3"); err == nil {
		t.Fatal("fixture tag leaked into the ambient GIT_DIR repository")
	}
}

// samePath 比较 git 与 Go 可能给出不同形式的路径（Windows 上 git 也返回正斜杠），
// 显式归一化分隔符后再比较，使断言针对仓库隔离而非路径格式。
func samePath(left, right string) bool {
	canonical := func(value string) string {
		if resolved, err := filepath.EvalSymlinks(value); err == nil {
			value = resolved
		}
		value = strings.ReplaceAll(value, `\`, "/")
		for strings.Contains(value, "//") {
			value = strings.ReplaceAll(value, "//", "/")
		}
		return strings.TrimSuffix(strings.ToLower(value), "/")
	}
	return canonical(left) == canonical(right)
}
