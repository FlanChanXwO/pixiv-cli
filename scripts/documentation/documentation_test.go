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

	internalpixiv "github.com/FlanChanXwO/pixiv-cli/internal/search/pixiv"
)

var markdownLinkPattern = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)

func TestLocalizedDocumentationLayout(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	required := []string{
		"README.md",
		"README.zh-CN.md",
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
		"docs/maintainers/architecture.md",
		"docs/maintainers/development.md",
		"docs/maintainers/plans/v1.0.0/index.md",
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

func TestJapaneseDocumentationFilesAreRemoved(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, relativePath := range []string{
		"README.ja.md",
		"docs/ja/cli-reference.md",
		"docs/ja/sdk.md",
		"docs/ja/mcp-tools.md",
	} {
		if _, err := os.Stat(filepath.Join(repositoryRoot, filepath.FromSlash(relativePath))); err == nil {
			t.Errorf("Japanese documentation file %s still exists", relativePath)
		} else if !os.IsNotExist(err) {
			t.Errorf("stat removed Japanese documentation file %s: %v", relativePath, err)
		}
	}
}

func TestMarkdownRelativeLinksResolve(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	candidates := []string{
		filepath.Join(repositoryRoot, "README.md"),
		filepath.Join(repositoryRoot, "README.zh-CN.md"),
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
	const languageMarker = "English / 中文"
	for relativePath, markers := range map[string][]string{
		".github/ISSUE_TEMPLATE/config.yml":          {"文档与使用问题"},
		".github/ISSUE_TEMPLATE/bug-report.yml":      {"发生了什么？"},
		".github/ISSUE_TEMPLATE/feature-request.yml": {"要解决的问题"},
		".github/PULL_REQUEST_TEMPLATE.md":           {"概述"},
	} {
		payload, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Errorf("read contribution template %s: %v", relativePath, err)
			continue
		}
		if !strings.Contains(string(payload), languageMarker) {
			t.Errorf("contribution template %s is missing language marker %q", relativePath, languageMarker)
		}
		if strings.Contains(string(payload), "日本語") {
			t.Errorf("contribution template %s still contains Japanese localization", relativePath)
		}
		for _, marker := range markers {
			if !strings.Contains(string(payload), marker) {
				t.Errorf("contribution template %s is missing localized marker %q", relativePath, marker)
			}
		}
	}
}

func TestIssueTemplateTitlesUseCompactPrefixes(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for relativePath, expectedTitle := range map[string]string{
		".github/ISSUE_TEMPLATE/bug-report.yml":      `title: "[Bug] "`,
		".github/ISSUE_TEMPLATE/feature-request.yml": `title: "[Feature] "`,
	} {
		payload, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Errorf("read issue template %s: %v", relativePath, err)
			continue
		}
		content := string(payload)
		if !strings.Contains(content, expectedTitle) {
			t.Errorf("issue template %s is missing compact title prefix %q", relativePath, expectedTitle)
		}
		if strings.Contains(content, `title: "[`) && strings.Contains(content, `]: "`) {
			t.Errorf("issue template %s still uses a colon after its title prefix", relativePath)
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
	} {
		payload, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatal(err)
		}
		for _, contract := range []string{
			"pixiv auth import [REFRESH_TOKEN]",
			"pixiv auth export [UID]",
			"pixiv timeline latest",
			"pixiv recommended",
			"pixiv mcp",
			"--ai-mode",
			"--resolution",
			"web_fallback_enabled",
		} {
			if !strings.Contains(string(payload), contract) {
				t.Errorf("%s is missing CLI contract %q", relativePath, contract)
			}
		}
		for _, obsolete := range []string{"pixiv feed", "search-options"} {
			if strings.Contains(string(payload), obsolete) {
				t.Errorf("%s still documents removed CLI command %q", relativePath, obsolete)
			}
		}
	}
}

func TestAuthMigrationDocumentationRequiresExplicitBundleTransfer(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	migration, err := os.ReadFile(filepath.Join(repositoryRoot, "docs", "en", "v1.0.0-migration.md"))
	if err != nil {
		t.Fatal(err)
	}
	migrationText := string(migration)
	for _, contract := range []string{
		"never reads or migrates the old `auth.json` automatically",
		"pixiv auth export --all --output <private bundle>",
		"pixiv auth import < bundle.json",
	} {
		if !strings.Contains(migrationText, contract) {
			t.Errorf("v1 migration guide is missing explicit auth transfer contract %q", contract)
		}
	}
	for _, relativePath := range []string{
		"skills/pixiv-cli/references/troubleshooting.md",
		"docs/en/cli-reference.md",
		"docs/zh-CN/cli-reference.md",
	} {
		payload, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatal(err)
		}
		content := string(payload)
		if strings.Contains(content, "migrated during startup") || strings.Contains(content, "migrates `auth.json` automatically") {
			t.Errorf("%s still promises automatic auth.json migration", relativePath)
		}
	}
}

func TestCLIReferenceLocalesHaveExactDrawingToolCatalog(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, relativePath := range []string{
		"docs/en/cli-reference.md",
		"docs/zh-CN/cli-reference.md",
	} {
		payload, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatal(err)
		}
		content := string(payload)
		position := 0
		for _, tool := range internalpixiv.SupportedDrawingTools() {
			next := strings.Index(content[position:], tool)
			if next < 0 {
				t.Errorf("%s is missing drawing-tool catalog value %q", relativePath, tool)
				break
			}
			position += next + len(tool)
		}
	}
}

func TestMCPToolDocumentsMatchTheRegisteredPublicSurface(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, relativePath := range []string{
		"docs/en/mcp-tools.md",
		"docs/zh-CN/mcp-tools.md",
	} {
		payload, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatal(err)
		}
		content := string(payload)
		for _, tool := range []string{
			"timeline_illust_latest", "timeline_novel_latest", "timeline_illust_following", "timeline_novel_following",
			"illust_filter", "novel_filter", "user_filter", "isError=true",
		} {
			if !strings.Contains(content, tool) {
				t.Errorf("%s is missing MCP contract %q", relativePath, tool)
			}
		}
		for _, removed := range []string{
			"set_download_path", "set_refresh_token", "search_illust_options", "illust_new", "novel_new",
		} {
			if strings.Contains(content, removed) {
				t.Errorf("%s still documents removed MCP tool %q", relativePath, removed)
			}
		}
	}
}

func TestSDKAndMCPDocumentationExposeSupportedLocales(t *testing.T) {
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
			localizedPath:   "docs/zh-CN/sdk.md",
			localizedTarget: "zh-CN/sdk.md",
			canonicalPaths:  []string{"docs/en/sdk.md"},
			requiredTerms: []string{
				"pixiv.Open",
				"sdk.Page",
				"CredentialsExpired",
				"malformed_upstream_response",
				"OpenResource",
				"fanbox.Open",
				"Resource",
			},
		},
		{
			indexPath:       "docs/index.md",
			localizedPath:   "docs/zh-CN/mcp-tools.md",
			localizedTarget: "zh-CN/mcp-tools.md",
			canonicalPaths:  []string{"docs/en/mcp-tools.md"},
			requiredTerms: []string{
				"search_novel",
				"search_user",
				"illust_ranking",
				"download_random_from_recommendation",
				"illust_recommended",
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
			t.Errorf("missing supported-language public contract %s: %v", contract.localizedPath, err)
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

func TestProductSkillRoutesInteractiveLoginToAuthReference(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	payload, err := os.ReadFile(filepath.Join(repositoryRoot, "skills", "pixiv-cli", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(payload)
	for _, requirement := range []string{
		"Before running `pixiv auth login`, read [`references/auth.md`]",
		"one-login hand-off URL",
		"transfers directly",
		"manual callback form.",
		"invent relay URLs or callback values.",
	} {
		if !strings.Contains(content, requirement) {
			t.Errorf("skills/pixiv-cli/SKILL.md is missing one-time hand-off contract %q", requirement)
		}
	}
	for _, removed := range []string{"**Login on this device**", "session page's manual form"} {
		if strings.Contains(content, removed) {
			t.Errorf("skills/pixiv-cli/SKILL.md still documents removed remote-login UI %q", removed)
		}
	}
}

func TestOneTimeRemoteLoginDocumentationContract(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, relativePath := range []string{
		"README.md",
		"README.zh-CN.md",
		"docs/en/cli-reference.md",
		"docs/zh-CN/cli-reference.md",
	} {
		payload, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatal(err)
		}
		content := string(payload)
		for _, requirement := range []string{
			"login_relay_public_url",
			"login_relay_listen_addr",
		} {
			if !strings.Contains(content, requirement) {
				t.Errorf("%s is missing one-time hand-off contract %q", relativePath, requirement)
			}
		}
		for _, removedCommand := range []string{
			"pixiv auth devices list",
			"pixiv auth devices revoke",
		} {
			if strings.Contains(content, removedCommand) {
				t.Errorf("%s still documents removed command %q", relativePath, removedCommand)
			}
		}
	}

	for _, relativePath := range []string{
		"docs/en/cli-reference.md",
		"docs/zh-CN/cli-reference.md",
	} {
		payload, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatal(err)
		}
		content := string(payload)
		if !strings.Contains(content, "pixiv://account/login") {
			t.Errorf("%s is missing the callback-handler protocol contract", relativePath)
		}
		if !strings.Contains(content, "pixiv://account/remote-login") {
			t.Errorf("%s is missing the desktop hand-off protocol contract", relativePath)
		}
		for _, removedTerm := range []string{
			"pixiv://account/pair",
			"pixiv auth devices list",
			"pixiv auth devices revoke",
		} {
			if strings.Contains(content, removedTerm) {
				t.Errorf("%s still documents removed remote-login surface %q", relativePath, removedTerm)
			}
		}
	}

	for _, contract := range []struct {
		relativePath string
		ignoredText  string
		removedText  string
	}{
		{relativePath: "docs/maintainers/adr/0009-cross-machine-login-relay.md", ignoredText: "完全忽略", removedText: "删除 `pixiv auth devices`"},
		{relativePath: "docs/maintainers/development.md", ignoredText: "会被忽略", removedText: "`pixiv auth devices` 已移除"},
	} {
		payload, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(contract.relativePath)))
		if err != nil {
			t.Fatal(err)
		}
		content := string(payload)
		for _, requirement := range []string{
			"remote-devices.json",
			"login_relay_secret",
			"login_relay_target_url",
		} {
			if !strings.Contains(content, requirement) {
				t.Errorf("%s is missing the ignored legacy relay state contract %q", contract.relativePath, requirement)
			}
		}
		if !strings.Contains(content, contract.ignoredText) {
			t.Errorf("%s no longer states that legacy relay state is ignored", contract.relativePath)
		}
		if !strings.Contains(content, contract.removedText) {
			t.Errorf("%s no longer states that auth devices was removed", contract.relativePath)
		}
	}

	skillPayload, err := os.ReadFile(filepath.Join(repositoryRoot, "skills", "pixiv-cli", "references", "auth.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, requirement := range []string{
		"one-time remote hand-off",
		"one-login hand-off URL",
		"redirects directly",
		"manually copied callback for this",
		"login_relay_secret",
		"login_relay_target_url",
		"pixiv auth devices",
		"silently ignored",
		"has been removed",
	} {
		if !strings.Contains(string(skillPayload), requirement) {
			t.Errorf("skills/pixiv-cli/references/auth.md is missing remote-login contract %q", requirement)
		}
	}
}

func TestRootReadmesLinkEverySupportedLocale(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, name := range []string{"README.md", "README.zh-CN.md"} {
		payload, err := os.ReadFile(filepath.Join(repositoryRoot, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, target := range []string{"README.md", "README.zh-CN.md"} {
			if !strings.Contains(string(payload), "]("+target+")") {
				t.Errorf("%s does not link locale %s", name, target)
			}
		}
	}
}
