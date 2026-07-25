// Command changescope classifies a Git diff for GitHub Actions CI routing.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	base := flag.String("base", "", "base Git commit")
	head := flag.String("head", "", "head Git commit")
	githubOutput := flag.String("github-output", "", "GitHub Actions output file")
	flag.Parse()

	docsOnly, reason, err := classify(*base, *head)
	if err != nil {
		fmt.Fprintf(os.Stderr, "classify change scope: %v\n", err)
		os.Exit(1)
	}
	if err := writeOutput(*githubOutput, docsOnly); err != nil {
		fmt.Fprintf(os.Stderr, "write change scope output: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, reason)
	fmt.Printf("docs_only=%t\n", docsOnly)
}

// classify 在缺少 push 的 before SHA 时明确选择完整验证。初始 push 没有可比较的
// 变更集，绝不能把它误判为文档改动而跳过二进制或供应链门禁。
func classify(base, head string) (bool, string, error) {
	if base == "" || isAllZero(base) {
		return false, "no usable base commit; selecting full validation", nil
	}
	if head == "" {
		return false, "", errors.New("head commit is required")
	}

	command := exec.Command("git", "diff", "--name-only", "--no-renames", "-z", base, head)
	output, err := command.Output()
	if err != nil {
		return false, "", fmt.Errorf("diff %s..%s: %w", base, head, err)
	}
	paths := splitNULPaths(output)
	if len(paths) == 0 {
		return false, "empty diff; selecting full validation", nil
	}
	if !docsOnlyPaths(paths) {
		return false, "non-document change detected; selecting full validation", nil
	}
	return true, "only approved documentation paths changed; selecting documentation validation", nil
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

// docsOnlyPaths 是刻意严格的 allowlist。任何代码、依赖、脚本、workflow 或未列出的
// 文件都会保留完整 CI，避免路径分类成为绕过发布验证的途径。
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
		(strings.HasPrefix(path, "README.") && strings.HasSuffix(path, ".md")) ||
		strings.HasPrefix(path, "docs/") ||
		strings.HasPrefix(path, "changelog/") ||
		strings.HasPrefix(path, "skills/")
}

func writeOutput(path string, docsOnly bool) error {
	if path == "" {
		return nil
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintf(file, "docs_only=%t\n", docsOnly)
	return err
}
