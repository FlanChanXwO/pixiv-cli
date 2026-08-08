package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2EScriptRunsCurrentSDKSuite(t *testing.T) {
	repoRoot := e2EScriptRepositoryRoot(t)
	goArgs := filepath.Join(t.TempDir(), "go-args")
	goEnv := filepath.Join(t.TempDir(), "go-env")
	fakeBin := writeE2EFakeGo(t)

	command := exec.Command("bash", filepath.Join(repoRoot, "scripts", "test-e2e.sh"))
	command.Dir = repoRoot
	command.Env = append(e2EScriptBaseEnvironment(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PIXIV_E2E_TEST_GO_ARGS="+goArgs,
		"PIXIV_E2E_TEST_GO_ENV="+goEnv,
		"PIXIV_E2E_PROXY=http://127.0.0.1:7890",
		"FANBOX_E2E_CREATOR_ID=creator-1",
		"FANBOX_E2E_TAG=tag-1",
		"FANBOX_E2E_POST_ID=post-1",
		"FANBOX_E2E_POST_URL=https://www.fanbox.cc/@creator-1/posts/post-1",
		"FANBOX_E2E_SOLVER_URL=http://127.0.0.1:8191",
		"FANBOX_E2E_SOLVER_PROXY=http://host.docker.internal:7890",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("test-e2e.sh invocation failed: %v\n%s", err, output)
	}

	if got, err := os.ReadFile(goArgs); err != nil || string(got) != "test\n./e2e\n-run\n^TestReal(Pixiv|Fanbox)SDKRead$\n-count=1\n-v\n" {
		t.Fatalf("go test arguments did not select the current SDK E2E suite")
	}
	env := readE2EScriptCapturedEnvironment(t, goEnv)
	for name, want := range map[string]string{
		"PIXIV_SDK_E2E":           "1",
		"FANBOX_SDK_E2E":          "1",
		"PIXIV_E2E_PROXY":         "http://127.0.0.1:7890",
		"FANBOX_E2E_CREATOR_ID":   "creator-1",
		"FANBOX_E2E_TAG":          "tag-1",
		"FANBOX_E2E_POST_ID":      "post-1",
		"FANBOX_E2E_POST_URL":     "https://www.fanbox.cc/@creator-1/posts/post-1",
		"FANBOX_E2E_SOLVER_URL":   "http://127.0.0.1:8191",
		"FANBOX_E2E_SOLVER_PROXY": "http://host.docker.internal:7890",
	} {
		if env[name] != want {
			t.Fatalf("captured %s = %q, want %q", name, env[name], want)
		}
	}
}

func TestE2EScriptSelectsPixivOnly(t *testing.T) {
	repoRoot := e2EScriptRepositoryRoot(t)
	goArgs := filepath.Join(t.TempDir(), "go-args")
	goEnv := filepath.Join(t.TempDir(), "go-env")
	fakeBin := writeE2EFakeGo(t)

	command := exec.Command("bash", filepath.Join(repoRoot, "scripts", "test-e2e.sh"), "--pixiv-only")
	command.Dir = repoRoot
	command.Env = append(e2EScriptBaseEnvironment(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PIXIV_E2E_TEST_GO_ARGS="+goArgs,
		"PIXIV_E2E_TEST_GO_ENV="+goEnv,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("test-e2e.sh --pixiv-only failed: %v\n%s", err, output)
	}
	if got, err := os.ReadFile(goArgs); err != nil || string(got) != "test\n./e2e\n-run\n^TestRealPixivSDKRead$\n-count=1\n-v\n" {
		t.Fatalf("go test arguments did not select only Pixiv SDK E2E")
	}
	env := readE2EScriptCapturedEnvironment(t, goEnv)
	if env["PIXIV_SDK_E2E"] != "1" || env["FANBOX_SDK_E2E"] != "" {
		t.Fatalf("current SDK E2E selection = %#v", env)
	}
}

func TestE2EScriptRejectsUnknownOptionBeforeGo(t *testing.T) {
	repoRoot := e2EScriptRepositoryRoot(t)
	goArgs := filepath.Join(t.TempDir(), "go-args")
	fakeBin := writeE2EFakeGo(t)
	command := exec.Command("bash", filepath.Join(repoRoot, "scripts", "test-e2e.sh"), "--unknown")
	command.Dir = repoRoot
	command.Env = append(e2EScriptBaseEnvironment(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PIXIV_E2E_TEST_GO_ARGS="+goArgs,
	)
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "Usage:") {
		t.Fatalf("test-e2e.sh accepted unknown option: err=%v output=%s", err, output)
	}
	if _, err := os.Stat(goArgs); !os.IsNotExist(err) {
		t.Fatal("test-e2e.sh invoked go after option validation failed")
	}
}

func e2EScriptRepositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func e2EScriptBaseEnvironment() []string {
	environment := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		name, _, found := strings.Cut(entry, "=")
		if found && (strings.HasPrefix(name, "PIXIV_E2E_") || strings.HasPrefix(name, "FANBOX_E2E_") || name == "PIXIV_SDK_E2E" || name == "FANBOX_SDK_E2E") {
			continue
		}
		environment = append(environment, entry)
	}
	return environment
}

func writeE2EFakeGo(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	path := filepath.Join(directory, "go")
	const program = `#!/bin/sh
printf '%s\n' "$@" > "$PIXIV_E2E_TEST_GO_ARGS"
{
	for name in PIXIV_SDK_E2E FANBOX_SDK_E2E PIXIV_E2E_PROXY FANBOX_E2E_CREATOR_ID FANBOX_E2E_TAG FANBOX_E2E_POST_ID FANBOX_E2E_POST_URL FANBOX_E2E_SOLVER_URL FANBOX_E2E_SOLVER_PROXY; do
    eval "value=\${$name-}"
    printf '%s=%s\n' "$name" "$value"
  done
} > "$PIXIV_E2E_TEST_GO_ENV"
`
	if err := os.WriteFile(path, []byte(program), 0o700); err != nil {
		t.Fatal(err)
	}
	return directory
}

func readE2EScriptCapturedEnvironment(t *testing.T, path string) map[string]string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	values := make(map[string]string)
	for line := range strings.SplitSeq(strings.TrimSuffix(string(body), "\n"), "\n") {
		name, value, found := strings.Cut(line, "=")
		if !found {
			t.Fatalf("captured environment has malformed entry")
		}
		values[name] = value
	}
	return values
}
