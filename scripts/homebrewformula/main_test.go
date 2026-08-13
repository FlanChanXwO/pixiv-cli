package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHomebrewFormulaEntrypointRequiresSubcommand(t *testing.T) {
	binary := buildHomebrewFormulaBinary(t)

	command := exec.Command(binary)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("homebrewformula with no subcommand succeeded: %s", output)
	}
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 1 {
		t.Fatalf("homebrewformula no-command exit = %v, want 1", err)
	}
	if !strings.Contains(string(output), "homebrew formula: a subcommand is required: render") {
		t.Fatalf("homebrewformula stderr = %q, want usage error", output)
	}
}

func buildHomebrewFormulaBinary(t *testing.T) string {
	t.Helper()
	repositoryRoot := findHomebrewFormulaRepositoryRoot(t)
	binaryName := "homebrewformula"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./scripts/homebrewformula")
	build.Dir = repositoryRoot
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./scripts/homebrewformula: %v\n%s", err, output)
	}
	return binaryPath
}

func findHomebrewFormulaRepositoryRoot(t *testing.T) string {
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
