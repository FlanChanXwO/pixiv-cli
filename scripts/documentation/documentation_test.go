package documentation_test

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
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
		"changelog/README.md",
		"changelog/README.zh-CN.md",
		"changelog/unreleased/en.md",
		"changelog/unreleased/zh-CN.md",
		"docs/en/cli-reference.md",
		"docs/en/sdk.md",
		"docs/en/mcp-tools.md",
		"docs/zh-CN/cli-reference.md",
		"docs/zh-CN/sdk.md",
		"docs/zh-CN/mcp-tools.md",
		"docs/ja/cli-reference.md",
		"docs/ja/sdk.md",
		"docs/ja/mcp-tools.md",
		"docs/maintainers/architecture.md",
		"docs/maintainers/development.md",
		"docs/maintainers/agents/documentation-guidelines.md",
		"docs/maintainers/adr/0011-localized-documentation-layout.md",
		".agents/skills/pixiv-cli-release-notes/SKILL.md",
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

	changelogRoot := filepath.Join(repositoryRoot, "changelog")
	err = filepath.WalkDir(changelogRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
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

func TestVersionedChangelogHasEnglishAndSimplifiedChinesePairs(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	changelogRoot := filepath.Join(repositoryRoot, "changelog")
	versions := []string{
		"0.1.0", "0.1.1", "0.2.0", "0.3.0", "0.4.0", "0.4.1", "0.4.2", "0.4.3",
		"0.4.4", "0.4.5", "0.5.0", "0.6.0", "0.7.0", "0.7.1", "0.7.2", "0.8.0",
	}
	previous := ""
	for _, version := range versions {
		directory := filepath.Join(changelogRoot, "v"+version)
		for _, locale := range []string{"en.md", "zh-CN.md"} {
			if info, err := os.Stat(filepath.Join(directory, locale)); err != nil {
				t.Errorf("v%s %s: %v", version, locale, err)
			} else if !info.Mode().IsRegular() {
				t.Errorf("v%s %s is not a regular file", version, locale)
			}
		}
		arguments := []string{"run", "./scripts/releasenotes", "validate", "--version", version, "--dir", filepath.ToSlash(filepath.Join("changelog", "v"+version))}
		if previous != "" {
			arguments = append(arguments, "--previous", previous)
		}
		command := exec.Command("go", arguments...)
		command.Dir = repositoryRoot
		if output, err := command.CombinedOutput(); err != nil {
			t.Errorf("validate v%s: %v\n%s", version, err, output)
		}
		previous = "v" + version
	}
	err := filepath.WalkDir(changelogRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "en.md" {
			return nil
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(path), "zh-CN.md")); err != nil {
			relative, _ := filepath.Rel(repositoryRoot, path)
			t.Errorf("%s has no matching Simplified Chinese changelog: %v", filepath.ToSlash(relative), err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, indexName := range []string{"README.md", "README.zh-CN.md"} {
		body, err := os.ReadFile(filepath.Join(changelogRoot, indexName))
		if err != nil {
			t.Fatal(err)
		}
		for index, version := range versions {
			previous := ""
			if index > 0 {
				previous = "v" + versions[index-1]
			}
			link := "https://github.com/FlanChanXwO/pixiv-cli/commits/v" + version
			if previous != "" {
				link = fmt.Sprintf("https://github.com/FlanChanXwO/pixiv-cli/compare/%s...v%s", previous, version)
			}
			if !strings.Contains(string(body), "| [v"+version+"]("+link+")") {
				t.Errorf("%s does not contain the expected v%s index link", indexName, version)
			}
		}
	}
}

func TestContributionTemplatesArePresentAndLocalized(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	const languageMarker = "English / 中文 / 日本語"
	for _, relativePath := range []string{
		".github/ISSUE_TEMPLATE/config.yml",
		".github/ISSUE_TEMPLATE/bug-report.yml",
		".github/ISSUE_TEMPLATE/feature-request.yml",
		".github/PULL_REQUEST_TEMPLATE.md",
	} {
		payload, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Errorf("read contribution template %s: %v", relativePath, err)
			continue
		}
		if !strings.Contains(string(payload), languageMarker) {
			t.Errorf("contribution template %s is missing language marker %q", relativePath, languageMarker)
		}
	}
}

func TestIssueTemplateTitlesAreLocalized(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for relativePath, title := range map[string]string{
		".github/ISSUE_TEMPLATE/bug-report.yml":      `title: "[Bug / 问题 / バグ]: "`,
		".github/ISSUE_TEMPLATE/feature-request.yml": `title: "[Feature / 功能 / 機能]: "`,
	} {
		payload, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Errorf("read issue template %s: %v", relativePath, err)
			continue
		}
		if !strings.Contains(string(payload), title) {
			t.Errorf("issue template %s is missing localized title %q", relativePath, title)
		}
	}
}

func TestPullRequestTemplateDeclaresReleaseNoteMetadata(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	payload, err := os.ReadFile(filepath.Join(repositoryRoot, ".github", "PULL_REQUEST_TEMPLATE.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		"<!-- release-note",
		"category:",
		"breaking:",
		"summary:",
		"none_reason:",
		"Added, Changed, Fixed, Security, Documentation, Maintenance, or None",
	} {
		if !strings.Contains(string(payload), field) {
			t.Errorf("pull-request template is missing release-note field or category %q", field)
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
			"pixiv auth import [REFRESH_TOKEN]",
			"pixiv auth export [UID]",
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

func TestSDKAndMCPDocumentationExposeJapaneseLocale(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, contract := range []struct {
		indexPath       string
		localizedPath   string
		localizedTarget string
		canonicalPaths  []string
		requiredTerms   []string
	}{
		{
			indexPath:       "docs/index.md",
			localizedPath:   "docs/ja/sdk.md",
			localizedTarget: "ja/sdk.md",
			canonicalPaths:  []string{"docs/en/sdk.md", "docs/zh-CN/sdk.md"},
			requiredTerms: []string{
				"SearchNovel",
				"IllustRankingRequest.Mode",
				"week_r18g",
				"download_url",
				"malformed_upstream_response",
				"LocalWriteCommitOutcome",
			},
		},
		{
			indexPath:       "docs/index.md",
			localizedPath:   "docs/ja/mcp-tools.md",
			localizedTarget: "ja/mcp-tools.md",
			canonicalPaths:  []string{"docs/en/mcp-tools.md", "docs/zh-CN/mcp-tools.md"},
			requiredTerms: []string{
				"search_novel",
				"search_user",
				"illust_ranking",
				"download_random_from_recommendation",
				"week_r18g",
			},
		},
	} {
		index, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(contract.indexPath)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(index), "("+contract.localizedTarget+")") {
			t.Errorf("%s does not link %s", contract.indexPath, contract.localizedTarget)
		}
		localized, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(contract.localizedPath)))
		if err != nil {
			t.Errorf("missing Japanese public contract %s: %v", contract.localizedPath, err)
		} else {
			for _, term := range contract.requiredTerms {
				if !strings.Contains(string(localized), term) {
					t.Errorf("%s is missing public contract %q", contract.localizedPath, term)
				}
			}
		}
		for _, canonicalPath := range contract.canonicalPaths {
			payload, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(canonicalPath)))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(payload), "../"+contract.localizedTarget) {
				t.Errorf("%s does not link %s", canonicalPath, contract.localizedTarget)
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
