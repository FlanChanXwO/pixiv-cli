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
		"{\"goos\":\"linux\",\"goarch\":\"amd64\",\"runner\":\"linux\",\"rust_target\":\"x86_64-unknown-linux-gnu\",\"rust_toolchain\":\"1.96.1\",\"cc\":\"gcc\",\"capabilities\":[\"smoke\",\"container\"]}," +
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
}
