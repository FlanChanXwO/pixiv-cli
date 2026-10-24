package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveFiltersCapabilityWithGoMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "platforms.json")
	body := "{\"platforms\":[" +
		"{\"goos\":\"linux\",\"goarch\":\"amd64\",\"runner\":\"linux\",\"capabilities\":[\"smoke\",\"container\",\"native-evidence\"]}," +
		"{\"goos\":\"darwin\",\"goarch\":\"arm64\",\"runner\":\"mac\",\"capabilities\":[\"smoke\"]}" +
		"]}"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := resolve(path, "container")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Include) != 1 || result.Include[0]["artifact"] != "linux-amd64" || result.Include[0]["runner"] != "linux" {
		t.Fatalf("unexpected matrix: %#v", result)
	}
	native, err := resolve(path, "native-evidence")
	if err != nil {
		t.Fatal(err)
	}
	if len(native.Include) != 1 || native.Include[0]["artifact"] != "linux-amd64" {
		t.Fatalf("unexpected native evidence matrix: %#v", native)
	}
}

func TestCheckedInNativeEvidenceUsesGoOnlyMetadata(t *testing.T) {
	t.Parallel()

	result, err := resolve(filepath.Join("..", "ci", "platforms.json"), "native-evidence")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Include) != 6 {
		t.Fatalf("native evidence targets = %d, want 6", len(result.Include))
	}
	windows := 0
	for _, item := range result.Include {
		if item["goos"] == "windows" {
			windows++
		}
		for _, key := range []string{"rust_target", "rust_toolchain", "cc"} {
			if _, exists := item[key]; exists {
				t.Fatalf("pure Go matrix retains %s", key)
			}
		}
	}
	if windows != 2 {
		t.Fatalf("Windows native evidence targets = %d, want 2", windows)
	}
}
