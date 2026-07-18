package main

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
)

func TestPlatformSmokeWorkflowPolicy(t *testing.T) {
	if err := Validate("../../.github/workflows/platform-smoke.yml"); err != nil {
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
	if err := ValidateQuality("../../.github/workflows/ci.yml"); err != nil {
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
