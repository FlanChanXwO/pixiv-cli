package platformsmokeworkflow_test

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/platformsmokeworkflow"
)

func TestPlatformSmokeWorkflowPolicy(t *testing.T) {
	if err := platformsmokeworkflow.Validate("../../../.github/workflows/platform-smoke.yml"); err != nil {
		t.Fatal(err)
	}
}

func TestPlatformSmokeLocksPortableLinuxABI(t *testing.T) {
	payload, err := os.ReadFile("../../../.github/workflows/platform-smoke.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(payload)
	for _, required := range []string{
		"runner: ubuntu-22.04\n            goos: linux\n            goarch: amd64",
		"runner: ubuntu-22.04-arm\n            goos: linux\n            goarch: arm64",
		`go run ./scripts/cmd/linuxabi --binary "$binary"`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("platform smoke workflow missing Linux ABI contract %q", required)
		}
	}
}

func TestPlatformSmokePinsAuditedRustToolchains(t *testing.T) {
	payload, err := os.ReadFile("../../../.github/workflows/platform-smoke.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(payload)
	for _, required := range []string{
		"RUSTUP_TOOLCHAIN: ${{ matrix.rust_toolchain }}",
		"runner: macos-15-intel\n            goos: darwin\n            goarch: amd64\n            rust_target: x86_64-apple-darwin\n            rust_toolchain: '1.96.0'",
		"runner: macos-15\n            goos: darwin\n            goarch: arm64\n            rust_target: aarch64-apple-darwin\n            rust_toolchain: '1.96.1'",
		"runner: ubuntu-22.04\n            goos: linux\n            goarch: amd64\n            rust_target: x86_64-unknown-linux-gnu\n            rust_toolchain: '1.96.1'",
		"runner: ubuntu-22.04-arm\n            goos: linux\n            goarch: arm64\n            rust_target: aarch64-unknown-linux-gnu\n            rust_toolchain: '1.96.1'",
		"runner: windows-2025\n            goos: windows\n            goarch: amd64\n            rust_target: x86_64-pc-windows-msvc\n            rust_toolchain: '1.96.0'",
		"runner: windows-11-arm\n            goos: windows\n            goarch: arm64\n            rust_target: aarch64-pc-windows-msvc\n            rust_toolchain: '1.96.1'",
		"rustup toolchain install '${{ matrix.rust_toolchain }}' --profile minimal --target '${{ matrix.rust_target }}' --no-self-update",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("platform smoke workflow missing pinned Rust contract %q", required)
		}
	}
}

func TestPlatformSmokeEmbedsOnlyRootVersion(t *testing.T) {
	payload, err := os.ReadFile("../../../.github/workflows/platform-smoke.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(payload)
	if !strings.Contains(workflow, `-ldflags "-X github.com/FlanChanXwO/pixiv-cli/internal/shared/buildinfo.Version=v${version}"`) {
		t.Fatal("platform smoke workflow must bind the root version")
	}
	for _, forbidden := range []string{"buildinfo.Commit", "buildinfo.BuildDate"} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("platform smoke workflow retains removed runtime metadata contract %q", forbidden)
		}
	}
}

func TestQualityWorkflowPolicy(t *testing.T) {
	if err := platformsmokeworkflow.ValidateQuality("../../../.github/workflows/ci.yml"); err != nil {
		t.Fatal(err)
	}
}

// 发布必须由现有受保护 Release workflow 在质量门禁后执行；不能由 changelog push
// 自动创建 tag 或 Release，否则会绕过同 SHA 的跨平台验收。
func TestNoAutomaticChangelogReleaseWorkflow(t *testing.T) {
	_, err := os.Stat("../../../.github/workflows/release-from-changelog.yml")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("automatic changelog release workflow must not exist: %v", err)
	}
}
