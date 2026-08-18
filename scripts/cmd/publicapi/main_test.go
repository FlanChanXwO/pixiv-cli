package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func buildPublicAPIBinary(t *testing.T) string {
	t.Helper()
	repositoryRoot := findPublicAPIRepositoryRoot(t)
	binaryName := "publicapi"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./scripts/cmd/publicapi")
	build.Dir = repositoryRoot
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./scripts/cmd/publicapi: %v\n%s", err, output)
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

func TestPublicAPICheckFailsWhenGoldenDrifts(t *testing.T) {
	binary := buildPublicAPIBinary(t)

	directory := t.TempDir()
	sdkDir := filepath.Join(directory, "sdk")
	if err := os.MkdirAll(sdkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sdkDir, "sdk.go"), []byte(`package sdk
func Exported() {}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	staleGolden := filepath.Join(t.TempDir(), "public-api-inventory.md")
	if err := os.WriteFile(staleGolden, []byte("## sdk\n- Different\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(binary, "--dir", directory, "--check", "--golden", staleGolden)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("check against drifted golden must exit non-zero, output=%q", output)
	}
	if !strings.Contains(string(output), "drifted from golden") {
		t.Fatalf("drift failure output = %q, want 'drifted from golden'", output)
	}

	correctGolden := filepath.Join(t.TempDir(), "public-api-inventory.md")
	render := exec.Command(binary, "--dir", directory)
	renderOutput, renderErr := render.CombinedOutput()
	if renderErr != nil {
		t.Fatalf("render inventory: %v\n%s", renderErr, renderOutput)
	}
	if err := os.WriteFile(correctGolden, renderOutput, 0o644); err != nil {
		t.Fatal(err)
	}
	pass := exec.Command(binary, "--dir", directory, "--check", "--golden", correctGolden)
	passOutput, passErr := pass.CombinedOutput()
	if passErr != nil {
		t.Fatalf("check against matching golden must exit 0: %v\n%s", passErr, passOutput)
	}
	if !strings.Contains(string(passOutput), "matches golden") {
		t.Fatalf("match output = %q, want 'matches golden'", passOutput)
	}
}
