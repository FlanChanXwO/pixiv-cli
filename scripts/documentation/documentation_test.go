package documentation_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var markdownLinkPattern = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)

func TestLocalizedDocumentationLayout(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	required := []string{
		"README.md",
		"README.zh-CN.md",
		"README.ja.md",
		"docs/en/cli-reference.md",
		"docs/en/sdk.md",
		"docs/en/mcp-tools.md",
		"docs/zh-CN/cli-reference.md",
		"docs/zh-CN/sdk.md",
		"docs/zh-CN/mcp-tools.md",
		"docs/ja/cli-reference.md",
		"docs/maintainers/architecture.md",
		"docs/maintainers/development.md",
		"docs/maintainers/agents/documentation-guidelines.md",
		"docs/maintainers/adr/0011-localized-documentation-layout.md",
	}
	for _, relativePath := range required {
		if info, err := os.Stat(filepath.Join(repositoryRoot, filepath.FromSlash(relativePath))); err != nil {
			t.Errorf("required documentation %s: %v", relativePath, err)
		} else if !info.Mode().IsRegular() {
			t.Errorf("required documentation %s is not a regular file", relativePath)
		}
	}
}

func TestMarkdownRelativeLinksResolve(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	candidates := []string{
		filepath.Join(repositoryRoot, "README.md"),
		filepath.Join(repositoryRoot, "README.zh-CN.md"),
		filepath.Join(repositoryRoot, "README.ja.md"),
		filepath.Join(repositoryRoot, "CONTRIBUTING.md"),
		filepath.Join(repositoryRoot, "CONTRIBUTING.zh-CN.md"),
	}

	docsRoot := filepath.Join(repositoryRoot, "docs")
	err := filepath.WalkDir(docsRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".understand-anything" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			candidates = append(candidates, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, documentPath := range candidates {
		payload, readErr := os.ReadFile(documentPath)
		if readErr != nil {
			t.Errorf("read %s: %v", documentPath, readErr)
			continue
		}
		content := string(payload)
		if strings.ContainsRune(content, '\uFFFD') {
			t.Errorf("%s contains a Unicode replacement character", documentPath)
		}
		if strings.Count(content, "```")%2 != 0 {
			t.Errorf("%s contains an unbalanced fenced code block", documentPath)
		}
		for _, match := range markdownLinkPattern.FindAllStringSubmatch(content, -1) {
			target := strings.TrimSpace(match[1])
			// 外部 URL 与同页 anchor 不属于仓库文件存在性检查。
			if target == "" || strings.HasPrefix(target, "#") || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			target = strings.Trim(target, "<>")
			if fragment := strings.IndexByte(target, '#'); fragment >= 0 {
				target = target[:fragment]
			}
			if target == "" || filepath.IsAbs(target) {
				continue
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(documentPath), filepath.FromSlash(target)))
			if _, statErr := os.Stat(resolved); statErr != nil {
				relativeDocument, _ := filepath.Rel(repositoryRoot, documentPath)
				t.Errorf("%s links to missing %q: %v", filepath.ToSlash(relativeDocument), match[1], statErr)
			}
		}
	}
}

func TestCLIReferenceLocalesExposeStableCommands(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, relativePath := range []string{
		"docs/en/cli-reference.md",
		"docs/zh-CN/cli-reference.md",
		"docs/ja/cli-reference.md",
	} {
		payload, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatal(err)
		}
		for _, contract := range []string{
			"pixiv auth token [UID]",
			"pixiv search-options",
			"pixiv recommended",
			"pixiv mcp",
			"--ai-mode",
			"--resolution",
			"web_fallback_enabled",
			"PIXIV_REFRESH_TOKEN",
		} {
			if !strings.Contains(string(payload), contract) {
				t.Errorf("%s is missing CLI contract %q", relativePath, contract)
			}
		}
	}
}

func TestRootReadmesLinkEverySupportedLocale(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, name := range []string{"README.md", "README.zh-CN.md", "README.ja.md"} {
		payload, err := os.ReadFile(filepath.Join(repositoryRoot, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, target := range []string{"README.md", "README.zh-CN.md", "README.ja.md"} {
			if !strings.Contains(string(payload), "]("+target+")") {
				t.Errorf("%s does not link locale %s", name, target)
			}
		}
	}
}
