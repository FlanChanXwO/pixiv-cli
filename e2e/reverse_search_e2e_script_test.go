package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReverseSearchE2EScriptRequiresOptInBeforeGo(t *testing.T) {
	repoRoot := e2EScriptRepositoryRoot(t)
	goArgs := filepath.Join(t.TempDir(), "go-args")
	fakeBin := writeReverseSearchFakeGo(t, "")

	command := exec.Command("bash", filepath.Join(repoRoot, "scripts", "test-reverse-search-e2e.sh"))
	command.Dir = repoRoot
	command.Env = append(reverseSearchScriptEnvironment(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PIXIV_E2E_TEST_GO_ARGS="+goArgs,
	)
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "PIXIV_REVERSE_SEARCH_E2E=1") {
		t.Fatalf("reverse-search e2e script did not require opt-in: err=%v output=%s", err, output)
	}
	if _, err := os.Stat(goArgs); !os.IsNotExist(err) {
		t.Fatal("reverse-search e2e script invoked go before opt-in")
	}
}

func TestReverseSearchE2EScriptSelectsRealTestWithoutPrintingSecrets(t *testing.T) {
	repoRoot := e2EScriptRepositoryRoot(t)
	goArgs := filepath.Join(t.TempDir(), "go-args")
	goEnv := filepath.Join(t.TempDir(), "go-env")
	fakeBin := writeReverseSearchFakeGo(t, goEnv)
	const source = "https://source-secret.example/image.png"
	const key = "api-key-secret"

	command := exec.Command("bash", filepath.Join(repoRoot, "scripts", "test-reverse-search-e2e.sh"))
	command.Dir = repoRoot
	command.Env = append(reverseSearchScriptEnvironment(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PIXIV_E2E_TEST_GO_ARGS="+goArgs,
		"PIXIV_E2E_TEST_GO_ENV="+goEnv,
		"PIXIV_REVERSE_SEARCH_E2E=1",
		"PIXIV_REVERSE_SEARCH_SOURCE="+source,
		"PIXIV_REVERSE_SEARCH_PROVIDER=all",
		"PIXIV_REVERSE_SEARCH_PROXY=http://127.0.0.1:7890",
		"SAUCENAO_API_KEY="+key,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("reverse-search e2e script failed: %v output=%s", err, output)
	}
	if got, err := os.ReadFile(goArgs); err != nil || string(got) != "test\n./e2e\n-run\n^TestRealReverseSearch$\n-count=1\n-v\n" {
		t.Fatalf("reverse-search e2e script selected unexpected go test arguments")
	}
	if strings.Contains(string(output), source) || strings.Contains(string(output), key) {
		t.Fatalf("reverse-search e2e script printed a sensitive value: %s", output)
	}
	env := readReverseSearchScriptEnvironment(t, goEnv)
	for name, want := range map[string]string{
		"PIXIV_REVERSE_SEARCH_E2E":      "1",
		"PIXIV_REVERSE_SEARCH_SOURCE":   source,
		"PIXIV_REVERSE_SEARCH_PROVIDER": "all",
		"PIXIV_REVERSE_SEARCH_PROXY":    "http://127.0.0.1:7890",
		"SAUCENAO_API_KEY":              key,
	} {
		if env[name] != want {
			t.Fatalf("captured %s did not reach the test process", name)
		}
	}
}

func reverseSearchScriptEnvironment() []string {
	environment := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		name, _, found := strings.Cut(entry, "=")
		if found && (strings.HasPrefix(name, "PIXIV_REVERSE_SEARCH_") || name == "SAUCENAO_API_KEY" || strings.HasPrefix(name, "PIXIV_E2E_TEST_GO_")) {
			continue
		}
		environment = append(environment, entry)
	}
	return environment
}

func writeReverseSearchFakeGo(t *testing.T, envPath string) string {
	t.Helper()
	directory := t.TempDir()
	path := filepath.Join(directory, "go")
	program := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$PIXIV_E2E_TEST_GO_ARGS\"\n"
	if envPath != "" {
		program += "{\n"
		for _, name := range []string{"PIXIV_REVERSE_SEARCH_E2E", "PIXIV_REVERSE_SEARCH_SOURCE", "PIXIV_REVERSE_SEARCH_PROVIDER", "PIXIV_REVERSE_SEARCH_PROXY", "SAUCENAO_API_KEY"} {
			program += "  eval \"value=\\${" + name + "-}\"\n"
			program += "  printf '%s=%s\\n' \"" + name + "\" \"$value\"\n"
		}
		program += "} > \"$PIXIV_E2E_TEST_GO_ENV\"\n"
	}
	if err := os.WriteFile(path, []byte(program), 0o700); err != nil {
		t.Fatal(err)
	}
	return directory
}

func readReverseSearchScriptEnvironment(t *testing.T, path string) map[string]string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	values := make(map[string]string)
	for line := range strings.SplitSeq(strings.TrimSuffix(string(body), "\n"), "\n") {
		name, value, found := strings.Cut(line, "=")
		if !found {
			t.Fatalf("captured reverse-search environment has malformed entry")
		}
		values[name] = value
	}
	return values
}
