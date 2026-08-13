package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPublicAPIEntrypointWritesInventoryToStdout(t *testing.T) {
	binary := buildPublicAPIBinary(t)

	command := exec.Command(binary, "--dir", filepath.Join(t.TempDir(), "missing"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("publicapi with missing dir failed: %v\n%s", err, output)
	}
	for _, want := range []string{"## sdk\n", "## sdk/pixiv\n", "## sdk/fanbox\n"} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("publicapi output = %q, missing %q", output, want)
		}
	}
}

func buildPublicAPIBinary(t *testing.T) string {
	t.Helper()
	repositoryRoot := findPublicAPIRepositoryRoot(t)
	binaryName := "publicapi"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./scripts/publicapi")
	build.Dir = repositoryRoot
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./scripts/publicapi: %v\n%s", err, output)
	}
	return binaryPath
}

func findPublicAPIRepositoryRoot(t *testing.T) string {
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
