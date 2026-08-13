package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestChangescopeEntrypointMapsExitCodesAndOutput(t *testing.T) {
	binary := buildScriptBinary(t, "changescope")

	t.Run("all-zero base selects full validation", func(t *testing.T) {
		command := exec.Command(binary, "--base", "0000000000000000000000000000000000000000", "--head", "0000000000000000000000000000000000000000")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("changescope with all-zero base failed: %v\n%s", err, output)
		}
		if !strings.Contains(string(output), "docs_only=false\n") {
			t.Fatalf("changescope stdout/stderr = %q, want docs_only=false", output)
		}
		if !strings.Contains(string(output), "no usable base commit") {
			t.Fatalf("changescope reason = %q, want no-usable-base reason", output)
		}
	})

	t.Run("missing head exits with error", func(t *testing.T) {
		command := exec.Command(binary, "--base", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		output, err := command.CombinedOutput()
		if err == nil {
			t.Fatalf("changescope with empty head succeeded: %s", output)
		}
		exit, ok := err.(*exec.ExitError)
		if !ok || exit.ExitCode() != 1 {
			t.Fatalf("changescope empty-head exit = %v, want 1", err)
		}
		if !strings.Contains(string(output), "classify change scope") {
			t.Fatalf("changescope stderr = %q, want classify error", output)
		}
	})
}

func buildScriptBinary(t *testing.T, name string) string {
	t.Helper()
	repositoryRoot := findScriptRepositoryRoot(t)
	binaryName := name
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./scripts/"+name)
	build.Dir = repositoryRoot
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./scripts/%s: %v\n%s", name, err, output)
	}
	return binaryPath
}

func findScriptRepositoryRoot(t *testing.T) string {
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
