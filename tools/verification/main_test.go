package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/tools/internal/verificationpolicy"
)

func TestBuiltinPrintfAndSanitize(t *testing.T) {
	var out cappedBuffer
	if err := builtinPrintf([]string{"%s\\n", "hello"}, &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "hello\n" {
		t.Fatalf("printf output = %q", got)
	}
	got := sanitize("Authorization: secret\nBearer abc.def.ghi\n")
	if strings.Contains(got, "secret") || strings.Contains(got, "abc.def.ghi") {
		t.Fatalf("sanitize leaked secret material: %q", got)
	}
}

func TestRunPipelineUsesRepositoryBinaryAndPipefail(t *testing.T) {
	if os.Getenv("GOOS") == "windows" {
		t.Skip("test helper uses a POSIX executable")
	}
	root := t.TempDir()
	binary := filepath.Join(root, "pixiv")
	body := "#!/bin/sh\ncat >/dev/null\nprintf 'ok\\n'\n"
	if err := os.WriteFile(binary, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	command, err := verificationpolicy.ParseLine("echo ABC-123 | pixiv detail")
	if err != nil {
		t.Fatal(err)
	}
	result := runPipeline(command, binary, root, time.Second)
	if result.Status != "passed" || result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "ok" {
		t.Fatalf("unexpected result: %#v", result)
	}

	failing := filepath.Join(root, "fail")
	if err := os.WriteFile(failing, []byte("#!/bin/sh\nexit 7\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	command, err = verificationpolicy.ParseLine("echo x | grep y | pixiv detail")
	if err != nil {
		t.Fatal(err)
	}
	// Force a middle-stage failure without relying on platform-specific grep
	// behavior by replacing PATH with the helper directory.
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", root+string(os.PathListSeparator)+oldPath)
	if err := os.Rename(failing, filepath.Join(root, "grep")); err != nil {
		t.Fatal(err)
	}
	result = runPipeline(command, binary, root, time.Second)
	if result.Status != "failed" || result.ExitCode != 7 {
		t.Fatalf("pipefail result = %#v", result)
	}
}

func TestRenderShowsOnlyFailedCommandDetails(t *testing.T) {
	expected := expectedMatrix{}
	expected.Include = append(expected.Include, struct {
		GOOS     string `json:"goos"`
		GOARCH   string `json:"goarch"`
		Artifact string `json:"artifact"`
	}{GOOS: "linux", GOARCH: "amd64", Artifact: "linux-amd64"})
	results := map[string]platformResult{
		"linux/amd64": {
			Platform: "linux/amd64",
			Commands: []commandResult{
				{Command: "pixiv --version", Status: "passed"},
				{Command: "pixiv detail ABC-123", Status: "failed", ExitCode: 1, DurationMS: 1200, Stages: []stageResult{{Command: "pixiv detail ABC-123", Status: "failed", ExitCode: 1, Stderr: "boom"}}},
			},
		},
	}
	body, overall := render(expected, results, "abcdef012345", "https://example.invalid/run")
	if overall != "failed" {
		t.Fatalf("overall = %q", overall)
	}
	if strings.Contains(body, "<code>pixiv --version</code>") {
		t.Fatal("successful command should not be expanded")
	}
	for _, want := range []string{"pixiv detail ABC-123", "Exit code", "1.2s", "boom", "View full GitHub Actions logs"} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered body missing %q", want)
		}
	}
}
