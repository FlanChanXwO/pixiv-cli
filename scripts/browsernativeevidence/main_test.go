package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBrowsernativeEvidenceEntrypointMapsExitCodes(t *testing.T) {
	binary := buildBrowsernativeEvidenceBinary(t)

	t.Run("usage exits with code 2", func(t *testing.T) {
		command := exec.Command(binary)
		output, err := command.CombinedOutput()
		if err == nil {
			t.Fatalf("browsernativeevidence with no args succeeded: %s", output)
		}
		exit, ok := err.(*exec.ExitError)
		if !ok || exit.ExitCode() != 2 {
			t.Fatalf("browsernativeevidence no-args exit = %v, want 2", err)
		}
		if !strings.Contains(string(output), "usage: browsernativeevidence policy --workflow PATH | firefox-contract --firefox PATH") {
			t.Fatalf("browsernativeevidence stderr = %q, want usage text", output)
		}
	})

	t.Run("policy failure exits with code 1", func(t *testing.T) {
		command := exec.Command(binary, "policy", "--workflow", filepath.Join(t.TempDir(), "missing.yml"))
		output, err := command.CombinedOutput()
		if err == nil {
			t.Fatalf("browsernativeevidence policy missing workflow succeeded: %s", output)
		}
		exit, ok := err.(*exec.ExitError)
		if !ok || exit.ExitCode() != 1 {
			t.Fatalf("browsernativeevidence policy exit = %v, want 1", err)
		}
		if !strings.Contains(string(output), "browser evidence: read workflow:") {
			t.Fatalf("browsernativeevidence stderr = %q, want read-workflow error", output)
		}
	})
}

func buildBrowsernativeEvidenceBinary(t *testing.T) string {
	t.Helper()
	repositoryRoot := findBrowsernativeEvidenceRepositoryRoot(t)
	binaryName := "browsernativeevidence"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./scripts/browsernativeevidence")
	build.Dir = repositoryRoot
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./scripts/browsernativeevidence: %v\n%s", err, output)
	}
	return binaryPath
}

func findBrowsernativeEvidenceRepositoryRoot(t *testing.T) string {
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
