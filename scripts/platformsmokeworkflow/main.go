package main

import (
	"fmt"
	"os"
	"strings"
)

// Validate 确保平台 smoke 继续验证已打包的二进制，而不是退化为只做跨平台编译。
func Validate(path string) error {
	workflow, err := readWorkflow(path)
	if err != nil {
		return err
	}
	for _, required := range []string{
		"pull_request:",
		"push:",
		"workflow_dispatch:",
		"permissions: {}",
		"macos-15-intel",
		"macos-15",
		"ubuntu-24.04",
		"ubuntu-24.04-arm",
		"windows-2025",
		"windows-11-arm",
		"go run ./scripts/releaseassets package",
		"go test ./scripts/installers -count=1",
		"PIXIV_E2E_BINARY=",
		"PIXIV_E2E_EXPECTED_VERSION=",
		"go test ./test/e2e -run '^TestPixivBinary' -count=1",
	} {
		if !strings.Contains(workflow, required) {
			return fmt.Errorf("platform smoke workflow missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"secrets.",
		"environment:",
		"PIXIV_E2E_REAL_API=1",
		"PIXIV_E2E_WEB_API=1",
	} {
		if strings.Contains(workflow, forbidden) {
			return fmt.Errorf("platform smoke workflow must not contain %q", forbidden)
		}
	}
	return validatePinnedActions(workflow)
}

// ValidateQuality 锁定常规 Linux 门禁，避免 PR 检查被缩减为只编译或只跑单元测试。
func ValidateQuality(path string) error {
	workflow, err := readWorkflow(path)
	if err != nil {
		return err
	}
	for _, required := range []string{
		"pull_request:",
		"push:",
		"workflow_dispatch:",
		"permissions: {}",
		"go test ./... -count=1",
		"go test -race ./... -count=1",
		"go vet ./...",
		"sh scripts/build.sh",
		"sh scripts/test-package-release.sh",
		"sh scripts/test-release-workflow.sh",
		"go test ./scripts/platformsmokeworkflow -count=1",
		"pre_commit run --all-files",
	} {
		if !strings.Contains(workflow, required) {
			return fmt.Errorf("quality workflow missing %q", required)
		}
	}
	for _, forbidden := range []string{"secrets.", "environment:", "PIXIV_E2E_REAL_API=1", "PIXIV_E2E_WEB_API=1"} {
		if strings.Contains(workflow, forbidden) {
			return fmt.Errorf("quality workflow must not contain %q", forbidden)
		}
	}
	return validatePinnedActions(workflow)
}

func readWorkflow(path string) (string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func validatePinnedActions(workflow string) error {
	for _, line := range strings.Split(workflow, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "uses: actions/") {
			continue
		}
		if at := strings.LastIndex(trimmed, "@"); at < 0 || len(trimmed[at+1:]) != 40 {
			return fmt.Errorf("workflow action is not pinned by full SHA: %q", trimmed)
		}
	}
	return nil
}
