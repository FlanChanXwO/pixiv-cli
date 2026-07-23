package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2EScriptRunsCompleteSuiteWithPositionalInputs(t *testing.T) {
	repoRoot := e2EScriptRepositoryRoot(t)
	goArgs := filepath.Join(t.TempDir(), "go-args")
	goEnv := filepath.Join(t.TempDir(), "go-env")
	fakeBin := writeE2EFakeGo(t)

	command := exec.Command("bash", filepath.Join(repoRoot, "scripts", "test-e2e.sh"),
		"test-refresh-token", "101", "202", "303", "illust word", "discovery word", "http://127.0.0.1:7890")
	command.Dir = repoRoot
	command.Env = append(e2EScriptBaseEnvironment(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PIXIV_E2E_TEST_GO_ARGS="+goArgs,
		"PIXIV_E2E_TEST_GO_ENV="+goEnv,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("test-e2e.sh positional invocation failed: %v\n%s", err, output)
	}

	if got, err := os.ReadFile(goArgs); err != nil || string(got) != "test\n./e2e\n-count=1\n-v\n" {
		t.Fatalf("go test arguments were not the complete e2e suite")
	}
	env := readE2EScriptCapturedEnvironment(t, goEnv)
	for name, want := range map[string]string{
		"PIXIV_E2E_REAL_API":           "1",
		"PIXIV_E2E_WEB_API":            "1",
		"PIXIV_E2E_SFW_ILLUST_ID":      "101",
		"PIXIV_E2E_R18_ILLUST_ID":      "202",
		"PIXIV_E2E_R18_UGOIRA_ID":      "303",
		"PIXIV_E2E_ILLUST_SEARCH_WORD": "illust word",
		"PIXIV_E2E_DISCOVERY_WORD":     "discovery word",
		"PIXIV_E2E_PROXY":              "http://127.0.0.1:7890",
		"PIXIV_WEB_API_PROXY":          "http://127.0.0.1:7890",
		"PIXIV_E2E_USE_LOCAL_AUTH":     "",
		"PIXIV_E2E_BINARY":             "",
	} {
		if env[name] != want {
			t.Fatalf("captured %s does not match the requested non-secret E2E configuration", name)
		}
	}
	if env["PIXIV_E2E_REFRESH_TOKEN"] == "" {
		t.Fatal("test-e2e.sh did not forward the explicit refresh token to the test process")
	}
}

func TestE2EScriptNonInteractiveRejectsMissingConfigurationBeforeGo(t *testing.T) {
	repoRoot := e2EScriptRepositoryRoot(t)
	goArgs := filepath.Join(t.TempDir(), "go-args")
	fakeBin := writeE2EFakeGo(t)
	command := exec.Command("bash", filepath.Join(repoRoot, "scripts", "test-e2e.sh"), "--non-interactive")
	command.Dir = repoRoot
	command.Env = append(e2EScriptBaseEnvironment(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PIXIV_E2E_TEST_GO_ARGS="+goArgs,
		"PIXIV_E2E_REFRESH_TOKEN=test-refresh-token",
		"PIXIV_E2E_SFW_ILLUST_ID=101",
		"PIXIV_E2E_R18_ILLUST_ID=202",
		"PIXIV_E2E_R18_UGOIRA_ID=303",
		"PIXIV_E2E_ILLUST_SEARCH_WORD=illust word",
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("test-e2e.sh accepted an incomplete non-interactive configuration")
	}
	if !strings.Contains(string(output), "PIXIV_E2E_DISCOVERY_WORD") {
		t.Fatalf("missing configuration diagnostic did not name the missing variable")
	}
	if _, err := os.Stat(goArgs); !os.IsNotExist(err) {
		t.Fatal("test-e2e.sh invoked go after configuration validation failed")
	}
}

func TestE2EScriptUsesHiddenTTYReadForRefreshToken(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(e2EScriptRepositoryRoot(t), "scripts", "test-e2e.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "read -r -s PIXIV_E2E_REFRESH_TOKEN") {
		t.Fatal("test-e2e.sh must use a hidden TTY read for a missing refresh token")
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
		if found && strings.HasPrefix(name, "PIXIV_E2E_") {
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
  for name in PIXIV_E2E_REAL_API PIXIV_E2E_WEB_API PIXIV_E2E_REFRESH_TOKEN PIXIV_E2E_SFW_ILLUST_ID PIXIV_E2E_R18_ILLUST_ID PIXIV_E2E_R18_UGOIRA_ID PIXIV_E2E_ILLUST_SEARCH_WORD PIXIV_E2E_DISCOVERY_WORD PIXIV_E2E_PROXY PIXIV_WEB_API_PROXY PIXIV_E2E_USE_LOCAL_AUTH PIXIV_E2E_BINARY; do
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
	for _, line := range strings.Split(strings.TrimSuffix(string(body), "\n"), "\n") {
		name, value, found := strings.Cut(line, "=")
		if !found {
			t.Fatalf("captured environment has malformed entry")
		}
		values[name] = value
	}
	return values
}
