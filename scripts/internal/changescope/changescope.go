// Package changescope classifies a Git diff for GitHub Actions CI routing.
package changescope

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Scope describes which heavyweight CI families must run for a change.
type Scope struct {
	DocsOnly          bool
	QualityRequired   bool
	PlatformRequired  bool
	ContainerRequired bool
	NativeRequired    bool
}

// Classify 在缺少 push 的 before SHA 时明确选择完整验证。初始 push 没有可比较的
// 变更集，绝不能把它误判为文档改动而跳过二进制或供应链门禁。
func Classify(base, head string) (Scope, string, error) {
	if base == "" || isAllZero(base) {
		return fullScope(true), "no usable base commit; selecting full validation", nil
	}
	if head == "" {
		return Scope{}, "", errors.New("head commit is required")
	}

	command := exec.Command("git", "diff", "--name-only", "--no-renames", "-z", base, head)
	output, err := command.Output()
	if err != nil {
		return Scope{}, "", fmt.Errorf("diff %s..%s: %w", base, head, err)
	}
	paths := splitNULPaths(output)
	if len(paths) == 0 {
		return fullScope(true), "empty diff; selecting full validation", nil
	}
	if docsOnlyPaths(paths) {
		return Scope{DocsOnly: true}, "only approved documentation paths changed; selecting documentation validation", nil
	}
	return fullScope(containerRelevant(paths)), "non-document change detected; selecting required validation", nil
}

func fullScope(container bool) Scope {
	return Scope{
		QualityRequired:   true,
		PlatformRequired:  true,
		ContainerRequired: container,
		NativeRequired:    true,
	}
}

func containerRelevant(paths []string) bool {
	for _, path := range paths {
		switch {
		case path == "Dockerfile", path == ".dockerignore", path == "go.mod", path == "go.sum",
			path == "Cargo.toml", path == "Cargo.lock":
			return true
		case strings.HasPrefix(path, "cmd/"),
			strings.HasPrefix(path, "internal/"),
			strings.HasPrefix(path, "sdk/"),
			strings.HasPrefix(path, "native/"),
			strings.HasPrefix(path, "ci/"),
			strings.HasPrefix(path, "tools/platformmatrix/"),
			path == ".github/workflows/container-smoke.yml",
			path == "scripts/build-platform.sh",
			strings.HasPrefix(path, "scripts/build-staticlibs"),
			strings.HasPrefix(path, "scripts/cmd/releaseassets/"):
			return true
		}
	}
	return false
}

func isAllZero(value string) bool {
	return strings.Trim(value, "0") == ""
}

func splitNULPaths(value []byte) []string {
	if len(value) == 0 {
		return nil
	}
	parts := strings.Split(string(value), "\x00")
	return parts[:len(parts)-1]
}

// docsOnlyPaths 是刻意严格的 allowlist。仅仓库说明、Agent 指令和不影响运行产物的
// 协作元数据可以跳过完整 CI；代码、依赖、构建/发布输入、workflow 和未列出的文件保留完整验证。
func docsOnlyPaths(paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	for _, path := range paths {
		if !isApprovedDocumentationPath(path) {
			return false
		}
	}
	return true
}

func isApprovedDocumentationPath(path string) bool {
	return path == "README.md" ||
		path == "AGENTS.md" ||
		path == "CLAUDE.md" ||
		path == ".gitignore" ||
		path == ".pre-commit-config.yaml" ||
		path == ".github/CODEOWNERS" ||
		path == ".github/PULL_REQUEST_TEMPLATE.md" ||
		(strings.HasPrefix(path, "README.") && strings.HasSuffix(path, ".md")) ||
		(strings.HasPrefix(path, "CONTRIBUTING.") && strings.HasSuffix(path, ".md")) ||
		path == "CONTRIBUTING.md" ||
		strings.HasPrefix(path, "docs/") ||
		strings.HasPrefix(path, "changelog/") ||
		strings.HasPrefix(path, "skills/") ||
		strings.HasPrefix(path, ".agents/skills/") ||
		strings.HasPrefix(path, ".github/ISSUE_TEMPLATE/")
}

// WriteOutput 追加写入 GitHub Actions 的输出文件；空路径表示跳过。
func WriteOutput(path string, scope Scope) error {
	if path == "" {
		return nil
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintf(file,
		"docs_only=%t\nquality_required=%t\nplatform_required=%t\ncontainer_required=%t\nnative_required=%t\n",
		scope.DocsOnly,
		scope.QualityRequired,
		scope.PlatformRequired,
		scope.ContainerRequired,
		scope.NativeRequired,
	)
	return err
}
