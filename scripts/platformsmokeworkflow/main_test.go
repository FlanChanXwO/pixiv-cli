package platformsmokeworkflow_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/platformsmokeworkflow"
)

func TestPlatformSmokeWorkflowPolicy(t *testing.T) {
	if err := platformsmokeworkflow.Validate("../../.github/workflows/platform-smoke.yml"); err != nil {
		t.Fatal(err)
	}
}

func TestPlatformSmokeLocksPortableLinuxABI(t *testing.T) {
	payload, err := os.ReadFile("../../.github/workflows/platform-smoke.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(payload)
	for _, required := range []string{
		"runner: ubuntu-22.04\n            goos: linux\n            goarch: amd64",
		"runner: ubuntu-22.04-arm\n            goos: linux\n            goarch: arm64",
		`go run ./scripts/linuxabi --binary "$binary"`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("platform smoke workflow missing Linux ABI contract %q", required)
		}
	}
}

func TestQualityWorkflowPolicy(t *testing.T) {
	if err := platformsmokeworkflow.ValidateQuality("../../.github/workflows/ci.yml"); err != nil {
		t.Fatal(err)
	}
}

// 发布必须由现有受保护 Release workflow 在质量门禁后执行；不能由 changelog push
// 自动创建 tag 或 Release，否则会绕过同 SHA 的跨平台验收。
func TestNoAutomaticChangelogReleaseWorkflow(t *testing.T) {
	_, err := os.Stat("../../.github/workflows/release-from-changelog.yml")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("automatic changelog release workflow must not exist: %v", err)
	}
}

// TestWorkflowGoPackagePathsExist 要求 workflow 里 go test/build/run 引用的每个
// 包路径都真实存在。
//
// 这条检查存在的原因：workflow 与其 policy 校验器都只比对字符串，包被移动后两者
// 会一起停留在旧路径而不报错，`go test` 只在 CI 上以 setup failure 失败——门禁看
// 起来在跑，实际什么都没验证。`internal/cli/auth/loginhelper` 迁到 Pixiv owner 后
// 就发生过这种漂移。
func TestWorkflowGoPackagePathsExist(t *testing.T) {
	entries, err := os.ReadDir("../../.github/workflows")
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`go (?:test|build|run) (\./[A-Za-z0-9_./-]+)`)
	checked := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}
		payload, err := os.ReadFile(filepath.Join("../../.github/workflows", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range pattern.FindAllStringSubmatch(string(payload), -1) {
			directory := strings.TrimSuffix(match[1], "/...")
			if directory == "." {
				continue
			}
			checked++
			info, err := os.Stat(filepath.Join("../..", directory))
			if err != nil || !info.IsDir() {
				t.Errorf("%s references a Go package path that does not exist: %s", entry.Name(), match[1])
			}
		}
	}
	if checked == 0 {
		t.Fatal("no Go package paths found in workflows; the extraction pattern is stale")
	}
}
