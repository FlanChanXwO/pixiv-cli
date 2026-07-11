package download

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// UgoiraStaticlibManifest 是 Task 13 生成的跨平台 staticlib 清单格式。
// 每个 artifact 都显式绑定 source identity、Rust target、仓库内的固定相对路径和内容摘要。
type UgoiraStaticlibManifest struct {
	Schema       int                                     `json:"schema"`
	SourceDigest string                                  `json:"source_digest"`
	Artifacts    map[string]UgoiraStaticlibManifestAsset `json:"artifacts"`
}

// UgoiraStaticlibManifestAsset 标识一个已生成 staticlib 的 Rust target 和 SHA-256。
type UgoiraStaticlibManifestAsset struct {
	Target string `json:"target"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

var ugoiraStaticlibPlatformTargets = map[string]string{
	"darwin/amd64":  "x86_64-apple-darwin",
	"darwin/arm64":  "aarch64-apple-darwin",
	"linux/amd64":   "x86_64-unknown-linux-gnu",
	"linux/arm64":   "aarch64-unknown-linux-gnu",
	"windows/amd64": "x86_64-pc-windows-msvc",
	"windows/arm64": "aarch64-pc-windows-msvc",
}

// CalculateUgoiraRustSourceDigest 计算参与 Rust encoder 产物的 Cargo/source 文件摘要。
// 不计 target/ 和测试源码，避免本机构建产物或测试变动伪造生产 source identity。
func CalculateUgoiraRustSourceDigest(crateDir, quantetteDir string) (string, error) {
	hasher := sha256.New()
	if err := hashUgoiraSourceTree(hasher, "ugoira_rs", crateDir, true); err != nil {
		return "", err
	}
	if err := hashUgoiraSourceTree(hasher, "quantette-0.6.0", quantetteDir, false); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func hashUgoiraSourceTree(hasher io.Writer, logicalRoot, directory string, includeLock bool) error {
	var files []string
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("ugoira source tree contains unsupported symlink %q", path)
		}
		if entry.IsDir() {
			if entry.Name() == "target" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}
		if isUgoiraDigestSource(rel, includeLock) {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("ugoira source tree %q contains no digest inputs", directory)
	}
	sort.Strings(files)
	for _, rel := range files {
		body, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		fileDigest := sha256.Sum256(body)
		if _, err := fmt.Fprintf(hasher, "%s/%s\x00%x\n", logicalRoot, rel, fileDigest); err != nil {
			return err
		}
	}
	return nil
}

func isUgoiraDigestSource(rel string, includeLock bool) bool {
	if rel == "Cargo.toml" || (includeLock && rel == "Cargo.lock") || rel == "build.rs" {
		return true
	}
	return strings.HasPrefix(rel, "src/") && strings.HasSuffix(rel, ".rs")
}

// ValidateUgoiraStaticlibManifest 严格校验 Task 13 的清单，锁住全部六个目标和源码身份。
func ValidateUgoiraStaticlibManifest(body []byte, sourceDigest string) error {
	if !isSHA256(sourceDigest) {
		return fmt.Errorf("expected ugoira source digest is not a SHA-256: %q", sourceDigest)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var manifest UgoiraStaticlibManifest
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("decode ugoira staticlib manifest: %w", err)
	}
	if err := ensureNoTrailingJSON(decoder); err != nil {
		return err
	}
	return validateUgoiraStaticlibManifest(manifest, sourceDigest)
}

// ValidateUgoiraStaticlibManifestFiles 同时验证清单格式与 staticlib 目录中真实文件的 SHA-256。
// 该函数只接受每个平台的固定布局，避免清单利用相对路径指向仓库外的任意文件。
func ValidateUgoiraStaticlibManifestFiles(staticlibDir string, body []byte, sourceDigest string) error {
	if err := ValidateUgoiraStaticlibManifest(body, sourceDigest); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var manifest UgoiraStaticlibManifest
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("decode ugoira staticlib manifest: %w", err)
	}
	for platform, asset := range manifest.Artifacts {
		path := filepath.Join(staticlibDir, filepath.FromSlash(asset.Path))
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("read ugoira staticlib for %q at %q: %w", platform, asset.Path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("ugoira staticlib for %q at %q is not a regular file", platform, asset.Path)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read ugoira staticlib for %q at %q: %w", platform, asset.Path, err)
		}
		digest := sha256.Sum256(body)
		if actual := hex.EncodeToString(digest[:]); actual != asset.SHA256 {
			return fmt.Errorf("ugoira staticlib SHA-256 for %q at %q = %q, want %q", platform, asset.Path, actual, asset.SHA256)
		}
	}
	return nil
}

func validateUgoiraStaticlibManifest(manifest UgoiraStaticlibManifest, sourceDigest string) error {
	if manifest.Schema != 1 {
		return fmt.Errorf("unsupported ugoira staticlib manifest schema %d", manifest.Schema)
	}
	if manifest.SourceDigest != sourceDigest {
		return fmt.Errorf("ugoira staticlib manifest source digest %q does not match %q", manifest.SourceDigest, sourceDigest)
	}
	if !isSHA256(manifest.SourceDigest) {
		return fmt.Errorf("ugoira staticlib manifest source digest is not a SHA-256: %q", manifest.SourceDigest)
	}
	if len(manifest.Artifacts) != len(ugoiraStaticlibPlatformTargets) {
		return fmt.Errorf("ugoira staticlib manifest has %d artifacts, want %d", len(manifest.Artifacts), len(ugoiraStaticlibPlatformTargets))
	}
	for platform, target := range ugoiraStaticlibPlatformTargets {
		asset, ok := manifest.Artifacts[platform]
		if !ok {
			return fmt.Errorf("ugoira staticlib manifest is missing platform %q", platform)
		}
		if asset.Target != target {
			return fmt.Errorf("ugoira staticlib manifest target for %q = %q, want %q", platform, asset.Target, target)
		}
		if want := expectedStaticlibManifestPath(platform, target); asset.Path != want {
			return fmt.Errorf("ugoira staticlib manifest path for %q = %q, want %q", platform, asset.Path, want)
		}
		if !isSHA256(asset.SHA256) {
			return fmt.Errorf("ugoira staticlib manifest SHA-256 for %q is invalid: %q", platform, asset.SHA256)
		}
	}
	for platform := range manifest.Artifacts {
		if _, ok := ugoiraStaticlibPlatformTargets[platform]; !ok {
			return fmt.Errorf("ugoira staticlib manifest has unsupported platform %q", platform)
		}
	}
	return nil
}

func expectedStaticlibManifestPath(platform, target string) string {
	archive := "libugoira_rs.a"
	if strings.HasPrefix(platform, "windows/") {
		archive = "ugoira_rs.lib"
	}
	return target + "/" + archive
}

func ensureNoTrailingJSON(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("ugoira staticlib manifest contains multiple JSON values")
		}
		return fmt.Errorf("read trailing ugoira staticlib manifest data: %w", err)
	}
	return nil
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && strings.ToLower(value) == value
}
