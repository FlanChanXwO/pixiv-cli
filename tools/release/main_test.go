package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpectedArtifactsFollowReleaseCapability(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(root, "platforms.json")
	body := "{\"platforms\":[" +
		"{\"goos\":\"linux\",\"goarch\":\"amd64\",\"capabilities\":[\"release\"]}," +
		"{\"goos\":\"linux\",\"goarch\":\"arm64\",\"capabilities\":[\"container\"]}," +
		"{\"goos\":\"windows\",\"goarch\":\"arm64\",\"capabilities\":[\"release\"]}" +
		"]}"
	if err := os.WriteFile(registryPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := expectedArtifacts(registryPath, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pixiv-cli_1.2.3_linux_amd64.tar.gz", "pixiv-cli_1.2.3_windows_arm64.zip"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestExpectedContainerArtifactsFollowContainerCapability(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(root, "platforms.json")
	body := "{\"platforms\":[" +
		"{\"goos\":\"linux\",\"goarch\":\"amd64\",\"capabilities\":[\"release\",\"container\"]}," +
		"{\"goos\":\"windows\",\"goarch\":\"arm64\",\"capabilities\":[\"release\"]}" +
		"]}"
	if err := os.WriteFile(registryPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := expectedContainerArtifacts(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "pixiv-cli-linux-amd64.tar" {
		t.Fatalf("unexpected container artifacts: %v", got)
	}
}
