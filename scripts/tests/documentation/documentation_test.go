// Package documentation_test 锁定面向用户的官方 Docker 使用契约。
//
// 这些测试在双语 README 更新前编写（Red 阶段）；它们只约束可复制执行的命令、
// 路径、镜像标签和安全语义，不要求两种语言逐句直译。
package documentation_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repositoryRoot 返回仓库根目录，供测试读取 canonical 英文与简体中文 README。
func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get current directory: %v", err)
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && info.Mode().IsRegular() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root")
		}
		dir = parent
	}
}

// readUserGuide 读取指定 README；不存在时直接失败。
func readUserGuide(t *testing.T, relativePath string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repositoryRoot(t), filepath.FromSlash(relativePath)))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	return string(body)
}

// requireFragments 确认文档包含稳定契约片段；fragment 保持英文命令与路径。
func requireFragments(t *testing.T, locale, document string, fragments []string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(document, fragment) {
			t.Fatalf("%s README must document Docker contract with %q", locale, fragment)
		}
	}
}

// rejectUnsafeClaims 防止文档宣称容器改变了既有产品模型或引入额外 registry。
func rejectUnsafeClaims(t *testing.T, locale, document string) {
	t.Helper()
	for _, forbidden := range []string{
		"Docker-specific product",
		"Docker-specific authentication",
		"Docker Hub",
		"docker.io/flanchanxwo",
	} {
		if strings.Contains(document, forbidden) {
			t.Fatalf("%s README must not claim or advertise %q", locale, forbidden)
		}
	}
}

// TestDockerInstallationContractIsBilingual 锁定安装、tag 选择、架构、状态与下载路径。
func TestDockerInstallationContractIsBilingual(t *testing.T) {
	t.Parallel()

	commonFragments := []string{
		"ghcr.io/flanchanxwo/pixiv-cli",
		"docker pull ghcr.io/flanchanxwo/pixiv-cli:",
		":v1.2.3",
		":latest",
		"linux/amd64",
		"linux/arm64",
		"-v pixiv-cli-state:/home/pixiv/.pixiv-cli",
		"-v \"$PWD:/work\"",
	}

	for locale, path := range map[string]string{
		"English":            "README.md",
		"Simplified Chinese": "README.zh-CN.md",
	} {
		document := readUserGuide(t, path)
		if !strings.Contains(document, "### Docker") {
			t.Fatalf("%s README must contain a dedicated Docker install section", locale)
		}
		requireFragments(t, locale, document, commonFragments)
		rejectUnsafeClaims(t, locale, document)
	}

	en := readUserGuide(t, "README.md")
	requireFragments(t, "English", en, []string{
		"`latest` follows stable releases only",
		"Prerelease tags never move `latest`",
	})
	zhCN := readUserGuide(t, "README.zh-CN.md")
	requireFragments(t, "Simplified Chinese", zhCN, []string{
		"`latest` 只跟随 stable release",
		"prerelease tag 绝不移动 `latest`",
	})
}

// TestDockerAuthMCPAndUpgradeContractIsBilingual 锁定凭据输入、MCP stdio 与拉取式升级。
func TestDockerAuthMCPAndUpgradeContractIsBilingual(t *testing.T) {
	t.Parallel()

	for locale, path := range map[string]string{
		"English":            "README.md",
		"Simplified Chinese": "README.zh-CN.md",
	} {
		document := readUserGuide(t, path)

		// auth import 必须走 stdin 和持久状态 volume；refresh token 不进入 argv 或镜像层。
		authPosition := strings.Index(document, "auth import")
		volumePosition := strings.Index(document, "-v pixiv-cli-state:/home/pixiv/.pixiv-cli")
		runPosition := strings.Index(document, "docker run --rm -i")
		if authPosition < 0 || volumePosition < 0 || runPosition < 0 || volumePosition > authPosition || runPosition > authPosition {
			t.Fatalf("%s README must recommend persistent-volume stdin-based auth import", locale)
		}

		// MCP 保持 stdio：交互式容器是契约的一部分，不宣传网络 transport。
		mcpCommand := "docker run --rm -i ghcr.io/flanchanxwo/pixiv-cli mcp"
		if !strings.Contains(document, mcpCommand) {
			t.Fatalf("%s README must document MCP stdio command %q", locale, mcpCommand)
		}

		// 升级语义必须基于 pull，且明确 updater 不感知容器部署。
		if !strings.Contains(document, "docker pull") {
			t.Fatalf("%s README must document pull-based container upgrades", locale)
		}
	}
	en := readUserGuide(t, "README.md")
	requireFragments(t, "English", en, []string{
		"Upgrade by pulling a newer image",
		"does not make `pixiv update` container-aware",
	})
	zhCN := readUserGuide(t, "README.zh-CN.md")
	requireFragments(t, "Simplified Chinese", zhCN, []string{
		"通过拉取新镜像升级",
		"不会将 `pixiv update` 改为 container-aware",
	})
	rejectUnsafeClaims(t, "English", en)
	rejectUnsafeClaims(t, "Simplified Chinese", zhCN)
}

// TestMaintainerDocsDocumentContainerReleaseVerification 锁定维护者文档中的
// 容器构建、发布边界和聚焦验证命令；公开 README 不重复这些维护者流程。
func TestMaintainerDocsDocumentContainerReleaseVerification(t *testing.T) {
	t.Parallel()

	contracts := map[string]struct {
		path      string
		heading   string
		fragments []string
	}{
		"English": {
			path:    "docs/en/maintainers/development.md",
			heading: "### Container release verification",
			fragments: []string{
				"`build_container` runs after the shared `build` gate and beside `build_production`",
				"`ubuntu-22.04` for `linux/amd64`",
				"`ubuntu-22.04-arm` for `linux/arm64`",
				"verified-container-linux-amd64",
				"verified-container-linux-arm64",
				"non-root",
				"`pixiv config path`",
				"org.opencontainers.image.source",
				"credential-free container smoke workflow",
				"go test ./scripts/internal/releaseworkflow -count=1",
				"go test ./scripts/tests/containerrelease -count=1",
				"go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml",
			},
		},
		"Simplified Chinese": {
			path:    "docs/zh-CN/maintainers/development.md",
			heading: "### 容器发布验证",
			fragments: []string{
				"`build_container` 在共享 `build` 门禁后运行，并与 `build_production` 并行",
				"`ubuntu-22.04` 对应 `linux/amd64`",
				"`ubuntu-22.04-arm` 对应 `linux/arm64`",
				"verified-container-linux-amd64",
				"verified-container-linux-arm64",
				"非 root",
				"`pixiv config path`",
				"org.opencontainers.image.source",
				"无凭据容器 smoke workflow",
				"go test ./scripts/internal/releaseworkflow -count=1",
				"go test ./scripts/tests/containerrelease -count=1",
				"go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml",
			},
		},
	}

	for locale, contract := range contracts {
		document := readUserGuide(t, contract.path)
		if !strings.Contains(document, contract.heading) {
			t.Fatalf("%s maintainer documentation must contain %q", locale, contract.heading)
		}
		for _, fragment := range contract.fragments {
			if !strings.Contains(document, fragment) {
				t.Fatalf("%s maintainer documentation must document container verification with %q", locale, fragment)
			}
		}
	}
}

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

func readDocumentation(t *testing.T, root, relativePath string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	return string(content)
}
