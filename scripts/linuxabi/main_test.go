package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLinuxABIEntrypointRequiresBinaryFlag(t *testing.T) {
	binary := buildLinuxABIBinary(t)

	command := exec.Command(binary)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("linuxabi with no binary flag succeeded: %s", output)
	}
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 1 {
		t.Fatalf("linuxabi no-binary exit = %v, want 1", err)
	}
	if !strings.Contains(string(output), "--binary is required") {
		t.Fatalf("linuxabi stderr = %q, want missing binary error", output)
	}
}

func buildLinuxABIBinary(t *testing.T) string {
	t.Helper()
	repositoryRoot := findLinuxABIRepositoryRoot(t)
	binaryName := "linuxabi"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./scripts/linuxabi")
	build.Dir = repositoryRoot
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./scripts/linuxabi: %v\n%s", err, output)
	}
	return binaryPath
}

func findLinuxABIRepositoryRoot(t *testing.T) string {
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
