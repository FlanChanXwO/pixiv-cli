package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUnderstandgraphEntrypointRequiresCommand(t *testing.T) {
	binary := buildUnderstandgraphBinary(t)

	command := exec.Command(binary)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("understandgraph with no command succeeded: %s", output)
	}
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 1 {
		t.Fatalf("understandgraph no-command exit = %v, want 1", err)
	}
	if !strings.Contains(string(output), "command is required") {
		t.Fatalf("understandgraph stderr = %q, want usage error", output)
	}
}

func buildUnderstandgraphBinary(t *testing.T) string {
	t.Helper()
	repositoryRoot := findUnderstandgraphRepositoryRoot(t)
	binaryName := "understandgraph"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./scripts/understandgraph")
	build.Dir = repositoryRoot
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./scripts/understandgraph: %v\n%s", err, output)
	}
	return binaryPath
}

func findUnderstandgraphRepositoryRoot(t *testing.T) string {
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
