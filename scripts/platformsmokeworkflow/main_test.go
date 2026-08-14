package platformsmokeworkflow_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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
	checked := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}
		payload, err := os.ReadFile(filepath.Join("../../.github/workflows", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, directory := range extractGoPackagePaths(string(payload)) {
			checked++
			info, err := os.Stat(filepath.Join("../..", directory))
			if err != nil || !info.IsDir() {
				t.Errorf("%s references a Go package path that does not exist: %s", entry.Name(), directory)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no Go package paths found in workflows; the extraction pattern is stale")
	}
}

// extractGoPackagePaths 从 workflow 文本中提取 `go <sub> [flags] <package...>`
// 的所有包参数。不做正则猜测：逐 token 扫描，跳过所有 `-flag` / `-flag=value`
// 形式的参数，`./...` 与包路径是剩余的参数。覆盖 `go test -race ./...`、
// `go build -trimpath -buildvcs=false ./cmd/pixiv` 这类带 flags 的写法。
func extractGoPackagePaths(payload string) []string {
	var paths []string
	tokens := strings.Fields(payload)
	for i := 0; i < len(tokens); i++ {
		if tokens[i] != "go" {
			continue
		}
		if i+1 >= len(tokens) || (tokens[i+1] != "test" && tokens[i+1] != "build" && tokens[i+1] != "run") {
			continue
		}
		for j := i + 2; j < len(tokens); j++ {
			token := tokens[j]
			if token == "go" {
				break
			}
			if strings.HasPrefix(token, "-") {
				continue
			}
			if !strings.HasPrefix(token, "./") && token != "." {
				// 非包参数（命令位置参数、test name、`-run` 的取值等）。
				// 只收集以 ./ 开头的包路径；`go test -run X pkg` 里 pkg
				// 也是包，但位置参数也可能是非包值，保守起见只收 ./ 前缀。
				continue
			}
			directory := strings.TrimSuffix(token, "/...")
			if directory == "." {
				continue
			}
			paths = append(paths, directory)
		}
	}
	return paths
}
