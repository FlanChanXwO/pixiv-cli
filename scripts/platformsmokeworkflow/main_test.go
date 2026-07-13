package main

import (
	"errors"
	"io/fs"
	"os"
	"testing"
)

func TestPlatformSmokeWorkflowPolicy(t *testing.T) {
	if err := Validate("../../.github/workflows/platform-smoke.yml"); err != nil {
		t.Fatal(err)
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
