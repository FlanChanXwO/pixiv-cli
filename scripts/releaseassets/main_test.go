package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseAssetsEntrypointMapsExitCodes(t *testing.T) {
	binary := buildReleaseAssetsBinary(t)

	command := exec.Command(binary)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("releaseassets with no subcommand succeeded: %s", output)
	}
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 1 {
		t.Fatalf("releaseassets no-command exit = %v, want 1", err)
	}
	if !strings.Contains(string(output), "release assets: a subcommand is required: validate, validate-source, channel, package, or finalize") {
		t.Fatalf("releaseassets stderr = %q, want usage error", output)
	}
}

func buildReleaseAssetsBinary(t *testing.T) string {
	t.Helper()
	repositoryRoot := findReleaseAssetsRepositoryRoot(t)
	binaryName := "releaseassets"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./scripts/releaseassets")
	build.Dir = repositoryRoot
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./scripts/releaseassets: %v\n%s", err, output)
	}
	return binaryPath
}

func findReleaseAssetsRepositoryRoot(t *testing.T) string {
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
