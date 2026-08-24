package documentation_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type documentationContract struct {
	path       string
	localePath string
	phrases    []string
}

func TestReverseSearchDocumentationAndLocaleRoutes(t *testing.T) {
	root := repositoryRoot(t)
	contracts := []documentationContract{
		{path: "README.md", localePath: "README.zh-CN.md", phrases: []string{
			"README.zh-CN.md", "reverse-image search", "docs/en/mcp-tools.md#reverse-image-search", "third-party providers",
		}},
		{path: "README.zh-CN.md", localePath: "README.md", phrases: []string{
			"README.md", "反向搜图", "docs/zh-CN/mcp-tools.md#反向搜图", "第三方 provider",
		}},
		{path: "docs/en/cli-reference.md", localePath: "../zh-CN/cli-reference.md", phrases: []string{
			"../zh-CN/cli-reference.md", "### Reverse image search", "reverse_search_provider", "saucenao_api_key", "SAUCENAO_API_KEY",
			"https://saucenao.com/legal.html", "type:\"artwork\"", "partial=true",
		}},
		{path: "docs/zh-CN/cli-reference.md", localePath: "../en/cli-reference.md", phrases: []string{
			"../en/cli-reference.md", "### 反向搜图", "reverse_search_provider", "saucenao_api_key", "SAUCENAO_API_KEY",
			"https://saucenao.com/legal.html", "type:\"artwork\"", "partial=true",
		}},
		{path: "docs/en/mcp-tools.md", localePath: "../zh-CN/mcp-tools.md", phrases: []string{
			"../zh-CN/mcp-tools.md", "## Reverse image search", "trusted-local-client model", "private, loopback, and link-local",
			"isError=false", "provider_errors", "records",
		}},
		{path: "docs/zh-CN/mcp-tools.md", localePath: "../en/mcp-tools.md", phrases: []string{
			"../en/mcp-tools.md", "## 反向搜图", "可信本机 client", "私网、loopback", "isError=false", "provider_errors", "records",
		}},
		{path: "docs/en/maintainers/architecture.md", localePath: "../../zh-CN/maintainers/architecture.md", phrases: []string{
			"../../zh-CN/maintainers/architecture.md", "### Reverse-search Facade exception", "private snapshot", "trusted local-client boundary",
		}},
		{path: "docs/zh-CN/maintainers/architecture.md", localePath: "../../en/maintainers/architecture.md", phrases: []string{
			"../../en/maintainers/architecture.md", "### reverse-search Facade 例外", "私有快照", "可信本机 client 边界",
		}},
		{path: "docs/en/maintainers/development.md", localePath: "../../zh-CN/maintainers/development.md", phrases: []string{
			"../../zh-CN/maintainers/development.md", "PIXIV_REVERSE_SEARCH_E2E=1", "scripts/test-reverse-search-e2e.sh", "SAUCENAO_API_KEY",
		}},
		{path: "docs/zh-CN/maintainers/development.md", localePath: "../../en/maintainers/development.md", phrases: []string{
			"../../en/maintainers/development.md", "PIXIV_REVERSE_SEARCH_E2E=1", "scripts/test-reverse-search-e2e.sh", "SAUCENAO_API_KEY",
		}},
		{path: "skills/pixiv-cli/SKILL.md", phrases: []string{
			"reverse-search images", "saucenao_api_key", "reverse_search_pixiv_only", "partial",
		}},
		{path: "skills/pixiv-cli/references/discover.md", phrases: []string{
			"## Reverse-search an image", "ascii2d-color", "generic `type=artwork`",
		}},
		{path: "skills/pixiv-cli/references/troubleshooting.md", phrases: []string{
			"Reverse search reports `missing_credential`", "partial=true", "Explicit `http:`/`https:`",
		}},
	}

	for _, contract := range contracts {
		content := readDocumentation(t, root, contract.path)
		for _, phrase := range contract.phrases {
			if !strings.Contains(content, phrase) {
				t.Errorf("%s is missing documentation contract phrase %q", contract.path, phrase)
			}
		}
		if contract.localePath == "" {
			continue
		}
		linkPath := filepath.Clean(filepath.Join(filepath.Dir(contract.path), contract.localePath))
		if _, err := os.Stat(filepath.Join(root, linkPath)); err != nil {
			t.Errorf("%s locale route %q does not resolve: %v", contract.path, contract.localePath, err)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
}

func readDocumentation(t *testing.T, root, relativePath string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	return string(content)
}
