package releaseassets

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInjectReleaseSourcesRendersVersionedInstallerBlock(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "install.sh")
	original := "before\n" + releaseSourcesBeginMarker + "release_sources='github-direct|{url}|{url}'\n" + releaseSourcesEndMarker + "\nafter\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	sources := []byte("proxy|-|https://proxy.example/{url}\ngithub-direct|{url}|{url}\n")
	sum, err := injectReleaseSources(path, sources)
	if err != nil {
		t.Fatalf("injectReleaseSources() error = %v", err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(payload)
	for _, required := range []string{
		"before\n",
		"release_sources=$(cat <<'" + releaseSourcesHeredoc + "'",
		"proxy|-|https://proxy.example/{url}",
		releaseSourcesEndMarker,
		"\nafter\n",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("rendered installer missing %q:\n%s", required, content)
		}
	}
	if strings.Contains(content, "release_sources='github-direct") {
		t.Fatalf("rendered installer retained template source block:\n%s", content)
	}
	if want := sha256.Sum256(payload); sum != want {
		t.Fatalf("rendered checksum = %x, want %x", sum, want)
	}
}

func TestInjectWindowsReleaseSourcesRendersIndexedCandidates(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "install.cmd")
	original := "before\r\n" + windowsReleaseSourcesBeginLine + "\r\nset \"RELEASE_SOURCE_COUNT=1\"\r\n" + windowsReleaseSourcesEndMarker + "\r\nafter\r\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := injectWindowsReleaseSources(path, []byte("proxy|https://proxy.example/{url}|https://proxy.example/{url}\ngithub-direct|{url}|{url}\n"))
	if err != nil {
		t.Fatalf("injectWindowsReleaseSources() error = %v", err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(payload)
	for _, required := range []string{
		"set \"RELEASE_SOURCE_COUNT=2\"",
		"set \"RELEASE_SOURCE_1=proxy|https://proxy.example/{url}|https://proxy.example/{url}\"",
		"set \"RELEASE_SOURCE_2=github-direct|{url}|{url}\"",
		windowsReleaseSourcesEndMarker,
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("rendered Windows installer missing %q:\n%s", required, content)
		}
	}
	if !strings.Contains(content, "\r\n") {
		t.Fatalf("rendered Windows installer lost CRLF bytes: %q", content)
	}
}
