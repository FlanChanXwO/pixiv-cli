package download

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUgoiraRustSourceDigestMatchesRecordedFixture(t *testing.T) {
	digest, err := CalculateUgoiraRustSourceDigest(
		filepath.Join("ugoira_rs"),
		filepath.Join("..", "..", "third_party", "rust", "quantette-0.6.0"),
	)
	if err != nil {
		t.Fatalf("CalculateUgoiraRustSourceDigest returned error: %v", err)
	}
	wantBytes, err := os.ReadFile(filepath.Join("testdata", "ugoira-source-digest.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.TrimSpace(string(wantBytes)); digest != want {
		t.Fatalf("Rust source digest = %s, want recorded %s", digest, want)
	}
}

func TestUgoiraRustSourceDigestTracksSourceButNotBuildArtifacts(t *testing.T) {
	root := t.TempDir()
	crateDir := filepath.Join(root, "ugoira_rs")
	quantetteDir := filepath.Join(root, "quantette")
	for path, body := range map[string]string{
		filepath.Join(crateDir, "Cargo.toml"):             "[package]\nname = \"ugoira_rs\"\nversion = \"0.1.0\"\n",
		filepath.Join(crateDir, "src", "lib.rs"):          "pub fn encode() {}\n",
		filepath.Join(crateDir, "target", "release", "x"): "first binary\n",
		filepath.Join(quantetteDir, "Cargo.toml"):         "[package]\nname = \"quantette\"\nversion = \"0.6.0\"\n",
		filepath.Join(quantetteDir, "src", "lib.rs"):      "pub fn quantize() {}\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	first, err := CalculateUgoiraRustSourceDigest(crateDir, quantetteDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(crateDir, "target", "release", "x"), []byte("second binary"), 0o644); err != nil {
		t.Fatal(err)
	}
	afterBinary, err := CalculateUgoiraRustSourceDigest(crateDir, quantetteDir)
	if err != nil {
		t.Fatal(err)
	}
	if afterBinary != first {
		t.Fatalf("build artifact changed source digest: first=%s after=%s", first, afterBinary)
	}
	if err := os.WriteFile(filepath.Join(crateDir, "src", "lib.rs"), []byte("pub fn changed() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	afterSource, err := CalculateUgoiraRustSourceDigest(crateDir, quantetteDir)
	if err != nil {
		t.Fatal(err)
	}
	if afterSource == first {
		t.Fatal("Rust source change did not change source digest")
	}
}

func TestValidateUgoiraStaticlibManifestAcceptsCompleteSyntheticMapping(t *testing.T) {
	sourceDigest := strings.Repeat("a", 64)
	if err := ValidateUgoiraStaticlibManifest([]byte(syntheticUgoiraStaticlibManifest(sourceDigest)), sourceDigest); err != nil {
		t.Fatalf("ValidateUgoiraStaticlibManifest returned error: %v", err)
	}
}

func TestValidateUgoiraStaticlibManifestFilesBindsPathsAndArtifactBytes(t *testing.T) {
	root := t.TempDir()
	sourceDigest := strings.Repeat("a", 64)
	manifest := UgoiraStaticlibManifest{
		Schema:       1,
		SourceDigest: sourceDigest,
		Artifacts:    make(map[string]UgoiraStaticlibManifestAsset, len(ugoiraStaticlibPlatformTargets)),
	}
	for platform, target := range ugoiraStaticlibPlatformTargets {
		path := expectedStaticlibManifestPath(platform, target)
		body := []byte("real staticlib for " + platform)
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, path), body, 0o644); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(body)
		manifest.Artifacts[platform] = UgoiraStaticlibManifestAsset{
			Target: target,
			Path:   path,
			SHA256: fmt.Sprintf("%x", digest),
		}
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateUgoiraStaticlibManifestFiles(root, body, sourceDigest); err != nil {
		t.Fatalf("ValidateUgoiraStaticlibManifestFiles returned error: %v", err)
	}

	darwinARM64 := manifest.Artifacts["darwin/arm64"]
	if err := os.WriteFile(filepath.Join(root, darwinARM64.Path), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateUgoiraStaticlibManifestFiles(root, body, sourceDigest); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("tampered staticlib error = %v, want SHA-256 error", err)
	}

	darwinARM64.Path = "../" + darwinARM64.Path
	manifest.Artifacts["darwin/arm64"] = darwinARM64
	traversal, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateUgoiraStaticlibManifest(traversal, sourceDigest); err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("traversal staticlib path error = %v, want path error", err)
	}
}

func TestCommittedUgoiraStaticlibManifestWhenPresent(t *testing.T) {
	staticlibDir := filepath.Join("ugoira_rs", "staticlib")
	body, err := os.ReadFile(filepath.Join(staticlibDir, "manifest.json"))
	if os.IsNotExist(err) {
		t.Skip("six-target staticlib manifest is intentionally absent until every native artifact is generated")
	}
	if err != nil {
		t.Fatal(err)
	}
	sourceDigest, err := CalculateUgoiraRustSourceDigest(
		filepath.Join("ugoira_rs"),
		filepath.Join("..", "..", "third_party", "rust", "quantette-0.6.0"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateUgoiraStaticlibManifestFiles(staticlibDir, body, sourceDigest); err != nil {
		t.Fatalf("ValidateUgoiraStaticlibManifestFiles returned error: %v", err)
	}
}

func TestValidateUgoiraStaticlibManifestRejectsInvalidMapping(t *testing.T) {
	sourceDigest := strings.Repeat("a", 64)
	for name, manifest := range map[string]string{
		"source digest mismatch": strings.Replace(syntheticUgoiraStaticlibManifest(sourceDigest), sourceDigest, strings.Repeat("b", 64), 1),
		"missing platform":       strings.Replace(syntheticUgoiraStaticlibManifest(sourceDigest), `,"windows/arm64":{"target":"aarch64-pc-windows-msvc","path":"aarch64-pc-windows-msvc/ugoira_rs.lib","sha256":"6666666666666666666666666666666666666666666666666666666666666666"}`, "", 1),
		"bad checksum":           strings.Replace(syntheticUgoiraStaticlibManifest(sourceDigest), `"sha256":"1111111111111111111111111111111111111111111111111111111111111111"`, `"sha256":"not-a-checksum"`, 1),
		"wrong target":           strings.Replace(syntheticUgoiraStaticlibManifest(sourceDigest), "x86_64-apple-darwin", "aarch64-apple-darwin", 1),
		"unknown field":          strings.Replace(syntheticUgoiraStaticlibManifest(sourceDigest), `{"schema":1,`, `{"schema":1,"unexpected":true,`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateUgoiraStaticlibManifest([]byte(manifest), sourceDigest); err == nil {
				t.Fatal("ValidateUgoiraStaticlibManifest returned nil error")
			}
		})
	}
}

func syntheticUgoiraStaticlibManifest(sourceDigest string) string {
	return fmt.Sprintf(`{"schema":1,"source_digest":"%s","artifacts":{"darwin/amd64":{"target":"x86_64-apple-darwin","path":"x86_64-apple-darwin/libugoira_rs.a","sha256":"1111111111111111111111111111111111111111111111111111111111111111"},"darwin/arm64":{"target":"aarch64-apple-darwin","path":"aarch64-apple-darwin/libugoira_rs.a","sha256":"2222222222222222222222222222222222222222222222222222222222222222"},"linux/amd64":{"target":"x86_64-unknown-linux-gnu","path":"x86_64-unknown-linux-gnu/libugoira_rs.a","sha256":"3333333333333333333333333333333333333333333333333333333333333333"},"linux/arm64":{"target":"aarch64-unknown-linux-gnu","path":"aarch64-unknown-linux-gnu/libugoira_rs.a","sha256":"4444444444444444444444444444444444444444444444444444444444444444"},"windows/amd64":{"target":"x86_64-pc-windows-msvc","path":"x86_64-pc-windows-msvc/ugoira_rs.lib","sha256":"5555555555555555555555555555555555555555555555555555555555555555"},"windows/arm64":{"target":"aarch64-pc-windows-msvc","path":"aarch64-pc-windows-msvc/ugoira_rs.lib","sha256":"6666666666666666666666666666666666666666666666666666666666666666"}}}`, sourceDigest)
}
