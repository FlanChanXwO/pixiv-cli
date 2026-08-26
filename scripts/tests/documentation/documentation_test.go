// Package documentation_test 锁定面向用户的官方 Docker 使用契约。
//
// 这些测试在双语 README 更新前编写（Red 阶段）；它们只约束可复制执行的命令、
// 路径、镜像标签和安全语义，不要求两种语言逐句直译。
package documentation_test

import (
	"os"
	"path/filepath"
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
