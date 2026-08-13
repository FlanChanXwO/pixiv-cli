package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseNotesEntrypointMapsExitCodes(t *testing.T) {
	binary := buildReleaseNotesBinary(t)

	command := exec.Command(binary)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("releasenotes with no subcommand succeeded: %s", output)
	}
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 1 {
		t.Fatalf("releasenotes no-command exit = %v, want 1", err)
	}
	if !strings.Contains(string(output), "a subcommand is required: validate, audit, prepare, pr-validate, or sync-history") {
		t.Fatalf("releasenotes stderr = %q, want usage error", output)
	}
}

func buildReleaseNotesBinary(t *testing.T) string {
	t.Helper()
	repositoryRoot := findReleaseNotesRepositoryRoot(t)
	binaryName := "releasenotes"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./scripts/releasenotes")
	build.Dir = repositoryRoot
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./scripts/releasenotes: %v\n%s", err, output)
	}
	return binaryPath
}

func findReleaseNotesRepositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if info, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil && info.Mode().IsRegular() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("could not find repository root")
		}
		directory = parent
	}
}
