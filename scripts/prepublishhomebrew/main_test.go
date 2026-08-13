package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPrepublishHomebrewEntrypointMapsExitCodes(t *testing.T) {
	binary := buildPrepublishHomebrewBinary(t)

	command := exec.Command(binary)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("prepublishhomebrew with no args succeeded: %s", output)
	}
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 1 {
		t.Fatalf("prepublishhomebrew no-args exit = %v, want 1", err)
	}
	if !strings.Contains(string(output), "prepublish Homebrew workflow policy: usage: prepublishhomebrew --workflow PATH") {
		t.Fatalf("prepublishhomebrew stderr = %q, want usage error", output)
	}
}

func buildPrepublishHomebrewBinary(t *testing.T) string {
	t.Helper()
	repositoryRoot := findPrepublishHomebrewRepositoryRoot(t)
	binaryName := "prepublishhomebrew"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./scripts/prepublishhomebrew")
	build.Dir = repositoryRoot
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./scripts/prepublishhomebrew: %v\n%s", err, output)
	}
	return binaryPath
}

func findPrepublishHomebrewRepositoryRoot(t *testing.T) string {
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
