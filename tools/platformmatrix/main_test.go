package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveFiltersCapabilityAndCarriesNativeMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "platforms.json")
	body := "{\"platforms\":[" +
		"{\"goos\":\"linux\",\"goarch\":\"amd64\",\"runner\":\"linux\",\"rust_target\":\"x86_64-unknown-linux-gnu\",\"rust_toolchain\":\"1.96.1\",\"cc\":\"gcc\",\"capabilities\":[\"smoke\",\"container\",\"native-evidence\"]}," +
		"{\"goos\":\"darwin\",\"goarch\":\"arm64\",\"runner\":\"mac\",\"rust_target\":\"aarch64-apple-darwin\",\"rust_toolchain\":\"1.96.1\",\"cc\":\"clang\",\"capabilities\":[\"smoke\"]}" +
		"]}"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := resolve(path, "container")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Include) != 1 || result.Include[0]["artifact"] != "linux-amd64" || result.Include[0]["rust_target"] != "x86_64-unknown-linux-gnu" {
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

func TestCheckedInWindowsNativeEvidenceUsesLLDBackedClang(t *testing.T) {
	t.Parallel()

	result, err := resolve(filepath.Join("..", "..", "ci", "platforms.json"), "native-evidence")
	if err != nil {
		t.Fatal(err)
	}
	windows := 0
	for _, item := range result.Include {
		if item["goos"] != "windows" {
			continue
		}
		windows++
		if got := item["cc"]; got != "clang -fuse-ld=lld" {
			t.Fatalf("Windows native evidence cc = %v, want clang -fuse-ld=lld", got)
		}
	}
	if windows != 2 {
		t.Fatalf("Windows native evidence targets = %d, want 2", windows)
	}
}
