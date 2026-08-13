package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLicenseBundleEntrypointMapsExitCodes(t *testing.T) {
	binary := buildLicenseBundleBinary(t)

	command := exec.Command(binary, "--repository", filepath.Join(t.TempDir(), "missing"))
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("licensebundle with missing repository succeeded: %s", output)
	}
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 1 {
		t.Fatalf("licensebundle missing-repository exit = %v, want 1", err)
	}
	if !strings.Contains(string(output), "license bundle:") {
		t.Fatalf("licensebundle stderr = %q, want license-bundle error prefix", output)
	}
}

func buildLicenseBundleBinary(t *testing.T) string {
	t.Helper()
	repositoryRoot := findLicenseBundleRepositoryRoot(t)
	binaryName := "licensebundle"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./scripts/licensebundle")
	build.Dir = repositoryRoot
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./scripts/licensebundle: %v\n%s", err, output)
	}
	return binaryPath
}

func findLicenseBundleRepositoryRoot(t *testing.T) string {
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
