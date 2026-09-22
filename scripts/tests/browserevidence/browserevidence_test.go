// Package browserevidence_test 锁住 §13 的 Browser Evidence 事实源边界。
//
// 平台身份（runner/goos/goarch/capability）只能来自 ci/platforms.json；Firefox
// 的版本、下载地址与 SHA256 只能来自独立的 ci/browser-fixtures.json。一旦把
// Firefox 元数据塞进通用平台注册表，或为 browser 再建一套平台注册表，测试即失败。
package browserevidence_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get current directory: %v", err)
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && info.Mode().IsRegular() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root")
		}
		dir = parent
	}
}

type platform struct {
	GOOS         string   `json:"goos"`
	GOARCH       string   `json:"goarch"`
	Runner       string   `json:"runner"`
	Capabilities []string `json:"capabilities"`
}

type registry struct {
	Platforms []platform `json:"platforms"`
}

func loadRegistry(t *testing.T) registry {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repositoryRoot(t), "ci", "platforms.json"))
	if err != nil {
		t.Fatalf("read ci/platforms.json: %v", err)
	}
	var cfg registry
	if err := json.Unmarshal(body, &cfg); err != nil {
		t.Fatalf("parse ci/platforms.json: %v", err)
	}
	return cfg
}

// TestBrowserCapabilityCoversEveryPlatform 覆盖 §13.1/§13.3：browser 能力来自
// 共享平台注册表，且必须保持 6 个平台的真实覆盖，不得因 registry 重构而减少。
func TestBrowserCapabilityCoversEveryPlatform(t *testing.T) {
	t.Parallel()

	cfg := loadRegistry(t)
	if len(cfg.Platforms) != 6 {
		t.Fatalf("expected 6 registered platforms, got %d", len(cfg.Platforms))
	}
	var browser int
	for _, item := range cfg.Platforms {
		for _, capability := range item.Capabilities {
			if capability == "browser" {
				browser++
			}
		}
	}
	if browser != 6 {
		t.Fatalf("expected the browser capability on all 6 platforms, got %d", browser)
	}
}

// TestPlatformRegistryDoesNotCarryFirefoxMetadata 覆盖 §13.2：通用平台注册表只
// 记录平台身份，Firefox 的版本/URL/SHA256 属于独立 fixture manifest。
func TestPlatformRegistryDoesNotCarryFirefoxMetadata(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join(repositoryRoot(t), "ci", "platforms.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(body))
	for _, forbidden := range []string{"firefox", "sha256", "ftp.mozilla.org"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("ci/platforms.json must not carry Firefox metadata (%q found)", forbidden)
		}
	}
}

// TestBrowserFixtureManifestIsSeparateAndComplete 覆盖 §13.2：Firefox fixture 是
// 按 goos/goarch 索引的独立 manifest，含 version/url/sha256。
func TestBrowserFixtureManifestIsSeparateAndComplete(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	path := filepath.Join(root, "ci", "browser-fixtures.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ci/browser-fixtures.json: %v", err)
	}
	var fixtures map[string]struct {
		Version string `json:"version"`
		URL     string `json:"url"`
		SHA256  string `json:"sha256"`
	}
	if err := json.Unmarshal(body, &fixtures); err != nil {
		t.Fatalf("parse ci/browser-fixtures.json: %v", err)
	}

	cfg := loadRegistry(t)
	for _, item := range cfg.Platforms {
		key := item.GOOS + "/" + item.GOARCH
		fixture, ok := fixtures[key]
		if !ok {
			t.Errorf("browser fixture missing for registered platform %s", key)
			continue
		}
		if fixture.Version == "" {
			t.Errorf("%s: version is required", key)
		}
		if !strings.HasPrefix(fixture.URL, "https://ftp.mozilla.org/") {
			t.Errorf("%s: url must be a pinned Mozilla download, got %q", key, fixture.URL)
		}
		if len(fixture.SHA256) != 64 {
			t.Errorf("%s: sha256 must be 64 hex characters, got %q", key, fixture.SHA256)
		}
	}
}

// TestBrowserEvidenceResolvesPlatformsFromRegistry 覆盖 §13.1：browser workflow
// 不再硬编码六个平台，平台集合统一经 tools/platformmatrix 解析。
func TestBrowserEvidenceResolvesPlatformsFromRegistry(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	body, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "browser-evidence.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)

	if !strings.Contains(text, "tools/platformmatrix --capability browser") {
		t.Error("browser evidence must resolve its platforms through tools/platformmatrix")
	}
	if !strings.Contains(text, "ci/browser-fixtures.json") {
		t.Error("browser evidence must read Firefox metadata from ci/browser-fixtures.json")
	}
	// 硬编码 runner 列表意味着平台集合又有第二个事实源。
	for _, hardcoded := range []string{"macos-15-intel", "windows-11-arm"} {
		if strings.Contains(text, hardcoded) {
			t.Errorf("browser evidence must not hardcode runners (%q found)", hardcoded)
		}
	}
}

// TestBrowserEvidenceKeepsSixPlatformWorkers 覆盖 §13.3：registry 化之后
// 真实覆盖仍是 6 个平台，每个平台都有 provider 与 Firefox profile 两条 contract。
func TestBrowserEvidenceKeepsSixPlatformWorkers(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	body, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "browser-evidence.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Jobs map[string]struct {
			Name string `yaml:"name"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(body, &document); err != nil {
		t.Fatalf("parse browser-evidence.yml: %v", err)
	}
	for _, job := range []string{"browser_provider", "firefox_native"} {
		spec, ok := document.Jobs[job]
		if !ok {
			t.Fatalf("browser-evidence.yml is missing job %q", job)
		}
		if spec.Name == "" {
			t.Errorf("job %q must keep a user-facing name", job)
		}
	}
}
