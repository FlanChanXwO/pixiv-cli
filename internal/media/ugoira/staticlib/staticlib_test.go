package staticlib_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira/staticlib"
)

var ugoiraStaticlibPlatformTargets = map[string]string{
	"darwin/amd64":  "x86_64-apple-darwin",
	"darwin/arm64":  "aarch64-apple-darwin",
	"linux/amd64":   "x86_64-unknown-linux-gnu",
	"linux/arm64":   "aarch64-unknown-linux-gnu",
	"windows/amd64": "x86_64-pc-windows-msvc",
	"windows/arm64": "aarch64-pc-windows-msvc",
}

func ugoiraStaticlibManifestPath(platform, target string) string {
	archive := "libugoira_rs.a"
	if strings.HasPrefix(platform, "windows/") {
		archive = "ugoira_rs.lib"
	}
	return target + "/" + archive
}

func TestUgoiraRustSourceDigestMatchesRecordedFixture(t *testing.T) {
	digest, err := staticlib.CalculateRustSourceDigest(
		filepath.Join("..", "rust"),
		filepath.Join("..", "..", "..", "..", "third_party", "rust", "quantette-0.6.0"),
	)
	if err != nil {
		t.Fatalf("staticlib.CalculateRustSourceDigest returned error: %v", err)
	}
	wantBytes, err := os.ReadFile(filepath.Join("..", "testdata", "ugoira-source-digest.txt"))
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
		filepath.Join(crateDir, "Cargo.toml"):                            "[package]\nname = \"ugoira_rs\"\nversion = \"0.1.0\"\n",
		filepath.Join(crateDir, ".cargo", "config.toml"):                 "[source.crates-io]\nreplace-with = \"vendored-sources\"\n",
		filepath.Join(crateDir, "src", "lib.rs"):                         "pub fn encode() {}\n",
		filepath.Join(crateDir, "vendor", "dep", ".cargo-checksum.json"): "{\"files\":{},\"package\":\"first\"}\n",
		filepath.Join(crateDir, "vendor", "dep", "src", "lib.rs"):        "pub fn vendored() {}\n",
		filepath.Join(crateDir, "target", "release", "x"):                "first binary\n",
		filepath.Join(quantetteDir, "Cargo.toml"):                        "[package]\nname = \"quantette\"\nversion = \"0.6.0\"\n",
		filepath.Join(quantetteDir, "src", "lib.rs"):                     "pub fn quantize() {}\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	first, err := staticlib.CalculateRustSourceDigest(crateDir, quantetteDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(crateDir, "target", "release", "x"), []byte("second binary"), 0o644); err != nil {
		t.Fatal(err)
	}
	afterBinary, err := staticlib.CalculateRustSourceDigest(crateDir, quantetteDir)
	if err != nil {
		t.Fatal(err)
	}
	if afterBinary != first {
		t.Fatalf("build artifact changed source digest: first=%s after=%s", first, afterBinary)
	}
	if err := os.WriteFile(filepath.Join(crateDir, ".cargo", "config.toml"), []byte("[source.crates-io]\nreplace-with = \"other-sources\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	afterConfig, err := staticlib.CalculateRustSourceDigest(crateDir, quantetteDir)
	if err != nil {
		t.Fatal(err)
	}
	if afterConfig == first {
		t.Fatal("Cargo source replacement change did not change source digest")
	}
	if err := os.WriteFile(filepath.Join(crateDir, "vendor", "dep", "src", "lib.rs"), []byte("pub fn vendored_changed() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	afterVendor, err := staticlib.CalculateRustSourceDigest(crateDir, quantetteDir)
	if err != nil {
		t.Fatal(err)
	}
	if afterVendor == afterConfig {
		t.Fatal("vendored Cargo source change did not change source digest")
	}
	if err := os.WriteFile(filepath.Join(crateDir, "src", "lib.rs"), []byte("pub fn changed() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	afterSource, err := staticlib.CalculateRustSourceDigest(crateDir, quantetteDir)
	if err != nil {
		t.Fatal(err)
	}
	if afterSource == afterVendor {
		t.Fatal("Rust source change did not change source digest")
	}
}

func TestValidateManifestAcceptsCompleteSyntheticMapping(t *testing.T) {
	sourceDigest := strings.Repeat("a", 64)
	if err := staticlib.ValidateManifest([]byte(syntheticManifest(sourceDigest)), sourceDigest); err != nil {
		t.Fatalf("staticlib.ValidateManifest returned error: %v", err)
	}
}

func TestValidateManifestFilesBindsPathsAndArtifactBytes(t *testing.T) {
	root := t.TempDir()
	sourceDigest := strings.Repeat("a", 64)
	manifest := staticlib.Manifest{
		Schema:       1,
		SourceDigest: sourceDigest,
		Artifacts:    make(map[string]staticlib.ManifestAsset, len(ugoiraStaticlibPlatformTargets)),
	}
	for platform, target := range ugoiraStaticlibPlatformTargets {
		path := ugoiraStaticlibManifestPath(platform, target)
		body := []byte("real staticlib for " + platform)
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, path), body, 0o644); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(body)
		manifest.Artifacts[platform] = staticlib.ManifestAsset{
			Target: target,
			Path:   path,
			SHA256: fmt.Sprintf("%x", digest),
		}
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := staticlib.ValidateManifestFiles(root, body, sourceDigest); err != nil {
		t.Fatalf("staticlib.ValidateManifestFiles returned error: %v", err)
	}

	darwinARM64 := manifest.Artifacts["darwin/arm64"]
	if err := os.WriteFile(filepath.Join(root, darwinARM64.Path), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := staticlib.ValidateManifestFiles(root, body, sourceDigest); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("tampered staticlib error = %v, want SHA-256 error", err)
	}

	darwinARM64.Path = "../" + darwinARM64.Path
	manifest.Artifacts["darwin/arm64"] = darwinARM64
	traversal, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := staticlib.ValidateManifest(traversal, sourceDigest); err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("traversal staticlib path error = %v, want path error", err)
	}
}

func TestCommittedManifestWhenPresent(t *testing.T) {
	staticlibDir := filepath.Join("..", "rust", "staticlib")
	body, err := os.ReadFile(filepath.Join(staticlibDir, "manifest.json"))
	if os.IsNotExist(err) {
		t.Skip("six-target staticlib manifest is intentionally absent until every native artifact is generated")
	}
	if err != nil {
		t.Fatal(err)
	}
	sourceDigest, err := staticlib.CalculateRustSourceDigest(
		filepath.Join("..", "rust"),
		filepath.Join("..", "..", "..", "..", "third_party", "rust", "quantette-0.6.0"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := staticlib.ValidateManifestFiles(staticlibDir, body, sourceDigest); err != nil {
		t.Fatalf("staticlib.ValidateManifestFiles returned error: %v", err)
	}
}

func TestValidateManifestRejectsInvalidMapping(t *testing.T) {
	sourceDigest := strings.Repeat("a", 64)
	for name, manifest := range map[string]string{
		"source digest mismatch": strings.Replace(syntheticManifest(sourceDigest), sourceDigest, strings.Repeat("b", 64), 1),
		"missing platform":       strings.Replace(syntheticManifest(sourceDigest), `,"windows/arm64":{"target":"aarch64-pc-windows-msvc","path":"aarch64-pc-windows-msvc/ugoira_rs.lib","sha256":"6666666666666666666666666666666666666666666666666666666666666666"}`, "", 1),
		"bad checksum":           strings.Replace(syntheticManifest(sourceDigest), `"sha256":"1111111111111111111111111111111111111111111111111111111111111111"`, `"sha256":"not-a-checksum"`, 1),
		"wrong target":           strings.Replace(syntheticManifest(sourceDigest), "x86_64-apple-darwin", "aarch64-apple-darwin", 1),
		"unknown field":          strings.Replace(syntheticManifest(sourceDigest), `{"schema":1,`, `{"schema":1,"unexpected":true,`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := staticlib.ValidateManifest([]byte(manifest), sourceDigest); err == nil {
				t.Fatal("staticlib.ValidateManifest returned nil error")
			}
		})
	}
}

func syntheticManifest(sourceDigest string) string {
	return fmt.Sprintf(`{"schema":1,"source_digest":"%s","artifacts":{"darwin/amd64":{"target":"x86_64-apple-darwin","path":"x86_64-apple-darwin/libugoira_rs.a","sha256":"1111111111111111111111111111111111111111111111111111111111111111"},"darwin/arm64":{"target":"aarch64-apple-darwin","path":"aarch64-apple-darwin/libugoira_rs.a","sha256":"2222222222222222222222222222222222222222222222222222222222222222"},"linux/amd64":{"target":"x86_64-unknown-linux-gnu","path":"x86_64-unknown-linux-gnu/libugoira_rs.a","sha256":"3333333333333333333333333333333333333333333333333333333333333333"},"linux/arm64":{"target":"aarch64-unknown-linux-gnu","path":"aarch64-unknown-linux-gnu/libugoira_rs.a","sha256":"4444444444444444444444444444444444444444444444444444444444444444"},"windows/amd64":{"target":"x86_64-pc-windows-msvc","path":"x86_64-pc-windows-msvc/ugoira_rs.lib","sha256":"5555555555555555555555555555555555555555555555555555555555555555"},"windows/arm64":{"target":"aarch64-pc-windows-msvc","path":"aarch64-pc-windows-msvc/ugoira_rs.lib","sha256":"6666666666666666666666666666666666666666666666666666666666666666"}}}`, sourceDigest)
}
