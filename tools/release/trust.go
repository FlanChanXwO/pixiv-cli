package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/releaseversion"
)

type sourceTrust struct {
	Version string
	Commit  string
}

type githubReleaseClient struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func verifySourceCommand(args []string) {
	set := flag.NewFlagSet("verify-source", flag.ExitOnError)
	repoRoot := set.String("repo-root", ".", "repository root")
	tag := set.String("tag", "", "immutable release tag")
	defaultBranch := set.String("default-branch", "", "default release branch")
	githubOutput := set.String("github-output", "", "optional GitHub Actions output file")
	_ = set.Parse(args)
	trust, err := verifySourceTrust(*repoRoot, *tag, *defaultBranch)
	if err != nil {
		fatal(err)
	}
	if *githubOutput != "" {
		if err := appendGitHubOutput(*githubOutput, map[string]string{
			"commit":  trust.Commit,
			"version": trust.Version,
		}); err != nil {
			fatal(err)
		}
	}
}

func verifyPublishedReleaseCommand(args []string) {
	set := flag.NewFlagSet("verify-published-release", flag.ExitOnError)
	repository := set.String("repository", "", "GitHub repository in owner/name form")
	tag := set.String("tag", "", "published release tag")
	stable := set.Bool("stable", false, "require a non-prerelease Release")
	_ = set.Parse(args)
	token := strings.TrimSpace(os.Getenv("GH_TOKEN"))
	if token == "" {
		fatal(errors.New("GH_TOKEN is required"))
	}
	client := githubReleaseClient{BaseURL: "https://api.github.com", Token: token, HTTP: http.DefaultClient}
	if err := verifyPublishedRelease(client, *repository, *tag, *stable); err != nil {
		fatal(err)
	}
}

func verifyHandoffCommand(args []string) {
	set := flag.NewFlagSet("verify-handoff", flag.ExitOnError)
	repository := set.String("repository", "", "GitHub repository in owner/name form")
	runID := set.String("run-id", "", "completed Release workflow run ID")
	workflow := set.String("workflow", "Release", "expected workflow name")
	repoRoot := set.String("repo-root", ".", "repository root containing the immutable tag")
	tag := set.String("tag", "", "immutable release tag")
	_ = set.Parse(args)
	parsedRunID, err := strconv.ParseInt(*runID, 10, 64)
	if err != nil || parsedRunID <= 0 {
		fatal(fmt.Errorf("run-id must be a positive decimal number: %q", *runID))
	}
	token := strings.TrimSpace(os.Getenv("GH_TOKEN"))
	if token == "" {
		fatal(errors.New("GH_TOKEN is required"))
	}
	tagCommit := ""
	if strings.TrimSpace(*tag) != "" {
		if _, err := releaseTagVersion(*tag); err != nil {
			fatal(err)
		}
		tagCommit, err = captureGit(*repoRoot, "rev-parse", "--verify", *tag+"^{commit}")
		if err != nil {
			fatal(fmt.Errorf("resolve handoff tag %q: %w", *tag, err))
		}
	}
	client := githubReleaseClient{BaseURL: "https://api.github.com", Token: token, HTTP: http.DefaultClient}
	if err := verifyReleaseHandoff(client, *repository, parsedRunID, *workflow, tagCommit); err != nil {
		fatal(err)
	}
}

func verifySourceTrust(repoRoot, tag, defaultBranch string) (sourceTrust, error) {
	version, err := releaseTagVersion(tag)
	if err != nil {
		return sourceTrust{}, err
	}
	if strings.TrimSpace(defaultBranch) == "" {
		return sourceTrust{}, errors.New("default branch is required")
	}
	absoluteRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return sourceTrust{}, err
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil {
		return sourceTrust{}, err
	}
	if !info.IsDir() {
		return sourceTrust{}, fmt.Errorf("repository root is not a directory: %s", absoluteRoot)
	}
	tagRef := "refs/tags/" + tag
	if err := runGit(absoluteRoot, "show-ref", "--verify", "--quiet", tagRef); err != nil {
		return sourceTrust{}, fmt.Errorf("release tag %q is unavailable: %w", tag, err)
	}
	commit, err := captureGit(absoluteRoot, "rev-parse", "--verify", tag+"^{commit}")
	if err != nil {
		return sourceTrust{}, fmt.Errorf("resolve release tag %q: %w", tag, err)
	}
	branchRef := "refs/remotes/origin/" + defaultBranch
	if err := runGit(absoluteRoot, "show-ref", "--verify", "--quiet", branchRef); err != nil {
		return sourceTrust{}, fmt.Errorf("default branch ref %q is unavailable: %w", branchRef, err)
	}
	if err := runGit(absoluteRoot, "merge-base", "--is-ancestor", commit, branchRef); err != nil {
		return sourceTrust{}, fmt.Errorf("release tag %q is not an ancestor of %s: %w", tag, branchRef, err)
	}
	return sourceTrust{Version: version, Commit: commit}, nil
}

func verifyPublishedRelease(client githubReleaseClient, repository, tag string, stable bool) error {
	version, err := releaseTagVersion(tag)
	if err != nil {
		return err
	}
	if stable && releaseversion.Channel(version) != "stable" {
		return fmt.Errorf("Release %q uses a prerelease semantic version, stable Release required", tag)
	}
	path, err := githubRepositoryPath(repository)
	if err != nil {
		return err
	}
	var release struct {
		TagName     string `json:"tag_name"`
		Draft       bool   `json:"draft"`
		Prerelease  bool   `json:"prerelease"`
		PublishedAt string `json:"published_at"`
	}
	if err := client.get(path+"/releases/tags/"+url.PathEscape(tag), &release); err != nil {
		return err
	}
	if release.TagName != tag {
		return fmt.Errorf("published Release tag %q does not match requested tag %q", release.TagName, tag)
	}
	if release.Draft || strings.TrimSpace(release.PublishedAt) == "" {
		return fmt.Errorf("Release %q is not published", tag)
	}
	if stable && release.Prerelease {
		return fmt.Errorf("Release %q is a prerelease, stable Release required", tag)
	}
	return nil
}

func verifyReleaseHandoff(client githubReleaseClient, repository string, runID int64, workflow, tagCommit string) error {
	path, err := githubRepositoryPath(repository)
	if err != nil {
		return err
	}
	if runID <= 0 {
		return errors.New("run ID must be positive")
	}
	if strings.TrimSpace(workflow) == "" {
		return errors.New("workflow name is required")
	}
	var run struct {
		Name       string `json:"name"`
		Conclusion string `json:"conclusion"`
		HeadSHA    string `json:"head_sha"`
	}
	if err := client.get(path+"/actions/runs/"+strconv.FormatInt(runID, 10), &run); err != nil {
		return err
	}
	if run.Name != workflow {
		return fmt.Errorf("handoff run workflow = %q, want %q", run.Name, workflow)
	}
	if run.Conclusion != "success" {
		return fmt.Errorf("handoff run conclusion = %q, want success", run.Conclusion)
	}
	if tagCommit != "" && run.HeadSHA != tagCommit {
		return fmt.Errorf("handoff run head SHA = %q, want tag commit %q", run.HeadSHA, tagCommit)
	}
	return nil
}

func releaseTagVersion(tag string) (string, error) {
	if !strings.HasPrefix(tag, "v") || len(tag) == 1 {
		return "", fmt.Errorf("release tag must be v-prefixed semantic version: %q", tag)
	}
	version := strings.TrimPrefix(tag, "v")
	if err := releaseversion.Validate(version); err != nil {
		return "", err
	}
	return version, nil
}

func githubRepositoryPath(repository string) (string, error) {
	owner, name, ok := strings.Cut(repository, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", fmt.Errorf("repository must use owner/name form: %q", repository)
	}
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name), nil
}

func (client githubReleaseClient) get(path string, destination any) error {
	if client.HTTP == nil {
		return errors.New("GitHub HTTP client is required")
	}
	if strings.TrimSpace(client.BaseURL) == "" {
		return errors.New("GitHub API base URL is required")
	}
	if strings.TrimSpace(client.Token) == "" {
		return errors.New("GitHub token is required")
	}
	request, err := http.NewRequest(http.MethodGet, strings.TrimRight(client.BaseURL, "/")+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+client.Token)
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "pixiv-cli-release-verifier")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("GitHub API %s returned %s: %s", path, response.Status, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		return fmt.Errorf("decode GitHub API %s: %w", path, err)
	}
	return nil
}

// gitEnv 返回针对指定仓库根运行 git 的环境变量。git 会让继承的
// GIT_DIR/GIT_WORK_TREE/GIT_INDEX_FILE 优先于 `-C`，而 pre-commit 在运行 hook
// 时会导出这三个变量；若不剥离，release 工具的 git 调用会直接作用在真实仓库上。
func gitEnv(repoRoot string) []string {
	env := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "GIT_") {
			continue
		}
		env = append(env, entry)
	}
	return append(env,
		"GIT_DIR="+filepath.Join(repoRoot, ".git"),
		"GIT_WORK_TREE="+repoRoot,
		"GIT_INDEX_FILE="+filepath.Join(repoRoot, ".git", "index"),
	)
}

func runGit(repoRoot string, args ...string) error {
	command := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	command.Env = gitEnv(repoRoot)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func captureGit(repoRoot string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	command.Env = gitEnv(repoRoot)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		return "", fmt.Errorf("git %s returned empty output", strings.Join(args, " "))
	}
	return value, nil
}

func appendGitHubOutput(path string, values map[string]string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	return appendGitHubOutputTo(file, values)
}

func appendGitHubOutputTo(file io.WriteCloser, values map[string]string) (retErr error) {
	defer func() {
		retErr = errors.Join(retErr, file.Close())
	}()
	for _, key := range []string{"commit", "version"} {
		value, ok := values[key]
		if !ok {
			continue
		}
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("GitHub output %s contains a newline", key)
		}
		if _, err := fmt.Fprintf(file, "%s=%s\n", key, value); err != nil {
			return err
		}
	}
	return nil
}
