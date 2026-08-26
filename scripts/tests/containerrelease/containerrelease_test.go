// Package containerrelease_test 验证 Dockerfile 和容器打包 contract。
//
// 这些测试在 Dockerfile 存在之前编写（Red 阶段），断言 Dockerfile 必须满足的
// 不可变 base digest、glibc runtime、非 root 用户、HOME/WORKDIR/ENTRYPOINT、
// OCI 元数据和无嵌入 secret 等约束。
package containerrelease_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// repositoryRoot 返回测试运行的仓库根目录。
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

// readDockerfile 读取仓库根目录的 Dockerfile；不存在时 t.Fatal。
func readDockerfile(t *testing.T) string {
	t.Helper()
	root := repositoryRoot(t)
	body, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatalf("Dockerfile must exist at repository root: %v", err)
	}
	return string(body)
}

// TestDockerfileExists 断言 Dockerfile 存在。
func TestDockerfileExists(t *testing.T) {
	t.Parallel()
	readDockerfile(t)
}

// TestDockerfileUsesImmutableBaseDigest 断言 FROM 指令使用不可变 digest
// 而非可变 tag（如 debian:bookworm-slim）。
func TestDockerfileUsesImmutableBaseDigest(t *testing.T) {
	t.Parallel()
	dockerfile := readDockerfile(t)

	// 匹配 FROM 行
	fromPattern := regexp.MustCompile(`(?m)^FROM\s+(\S+)`)
	matches := fromPattern.FindAllStringSubmatch(dockerfile, -1)
	if len(matches) == 0 {
		t.Fatal("Dockerfile must contain at least one FROM instruction")
	}
	for _, match := range matches {
		image := match[1]
		// 去掉可能的 AS alias 部分
		image = strings.TrimSpace(strings.Split(image, " AS ")[0])
		// 不可变 digest 格式：registry/image@sha256:<hex>
		// 或 image@sha256:<hex>（隐含 Docker Hub）
		if !regexp.MustCompile(`@sha256:[0-9a-f]{64}$`).MatchString(image) {
			t.Fatalf("FROM %q must use an immutable digest (sha256:hex), not a movable tag", image)
		}
	}
}

// TestDockerfileDoesNotUseAlpineMuslOrScratch 断言不使用 Alpine/musl 或 scratch base。
func TestDockerfileDoesNotUseAlpineMuslOrScratch(t *testing.T) {
	t.Parallel()
	dockerfile := readDockerfile(t)
	lowered := strings.ToLower(dockerfile)

	for _, forbidden := range []string{"alpine", "scratch"} {
		if strings.Contains(lowered, forbidden) {
			t.Fatalf("Dockerfile must not use %s as base image (glibc-based Debian slim required)", forbidden)
		}
	}
}

// TestDockerfileRunsAsNonRoot 断言最终镜像以非 root 用户运行。
func TestDockerfileRunsAsNonRoot(t *testing.T) {
	t.Parallel()
	dockerfile := readDockerfile(t)

	// 必须创建非 root 用户
	if !strings.Contains(dockerfile, "useradd") && !strings.Contains(dockerfile, "adduser") {
		t.Fatal("Dockerfile must create a non-root user")
	}
	// 必须使用 USER 指令切换到非 root 用户
	if !strings.Contains(dockerfile, "USER ") {
		t.Fatal("Dockerfile must switch to a non-root user via USER instruction")
	}
}

// TestDockerfileSetsHomeToPixiv 断言 HOME 设置为 /home/pixiv。
func TestDockerfileSetsHomeToPixiv(t *testing.T) {
	t.Parallel()
	dockerfile := readDockerfile(t)

	// 检查 ENV HOME=/home/pixiv 或 useradd --home-dir /home/pixiv
	if !strings.Contains(dockerfile, "/home/pixiv") {
		t.Fatal("Dockerfile must set HOME to /home/pixiv")
	}
	if !strings.Contains(dockerfile, "HOME=/home/pixiv") {
		t.Fatal("Dockerfile must set ENV HOME=/home/pixiv")
	}
}

// TestDockerfileSetsWorkdirToWork 断言 WORKDIR 设置为 /work。
func TestDockerfileSetsWorkdirToWork(t *testing.T) {
	t.Parallel()
	dockerfile := readDockerfile(t)

	if !strings.Contains(dockerfile, "WORKDIR /work") {
		t.Fatal("Dockerfile must set WORKDIR /work")
	}
}

// TestDockerfileSetsEntrypoint 断言 ENTRYPOINT 为 pixiv 二进制。
func TestDockerfileSetsEntrypoint(t *testing.T) {
	t.Parallel()
	dockerfile := readDockerfile(t)

	if !strings.Contains(dockerfile, `ENTRYPOINT ["/usr/local/bin/pixiv"]`) {
		t.Fatal(`Dockerfile must set ENTRYPOINT ["/usr/local/bin/pixiv"]`)
	}
}

// TestDockerfileCopiesPixivBinary 断言 COPY 或 ADD pixiv 二进制到 /usr/local/bin/pixiv。
func TestDockerfileCopiesPixivBinary(t *testing.T) {
	t.Parallel()
	dockerfile := readDockerfile(t)

	if !strings.Contains(dockerfile, "/usr/local/bin/pixiv") {
		t.Fatal("Dockerfile must copy the pixiv binary to /usr/local/bin/pixiv")
	}
}

// TestDockerfileHasOCILabels 断言 OCI provenance 元数据标签存在。
func TestDockerfileHasOCILabels(t *testing.T) {
	t.Parallel()
	dockerfile := readDockerfile(t)

	// OCI 标准标签
	requiredLabels := []string{
		"org.opencontainers.image.source",
		"org.opencontainers.image.revision",
		"org.opencontainers.image.version",
		"org.opencontainers.image.license",
	}
	for _, label := range requiredLabels {
		if !strings.Contains(dockerfile, label) {
			t.Fatalf("Dockerfile must include OCI label %q", label)
		}
	}
}

// TestDockerfileInstallsCACertificates 断言安装 CA 证书以支持 HTTPS 连接。
func TestDockerfileInstallsCACertificates(t *testing.T) {
	t.Parallel()
	dockerfile := readDockerfile(t)

	if !strings.Contains(dockerfile, "ca-certificates") {
		t.Fatal("Dockerfile must install ca-certificates for HTTPS connections")
	}
}

// TestDockerfileDoesNotEmbedSecretsOrState 断言 Dockerfile 不嵌入 secret 或本地状态文件。
func TestDockerfileDoesNotEmbedSecretsOrState(t *testing.T) {
	t.Parallel()
	dockerfile := readDockerfile(t)
	lowered := strings.ToLower(dockerfile)

	for _, forbidden := range []string{
		"refresh_token",
		"access_token",
		"secret",
		".pixiv-cli/",
		"token.json",
		"credentials",
	} {
		if strings.Contains(lowered, forbidden) {
			t.Fatalf("Dockerfile must not embed secrets or local state (found %q)", forbidden)
		}
	}
}

// TestDockerfileUsesDebianSlimBase 断言 base image 是 Debian slim（glibc-based）。
func TestDockerfileUsesDebianSlimBase(t *testing.T) {
	t.Parallel()
	dockerfile := readDockerfile(t)
	lowered := strings.ToLower(dockerfile)

	if !strings.Contains(lowered, "debian") {
		t.Fatal("Dockerfile must use a Debian slim base image (glibc-based)")
	}
	if !strings.Contains(lowered, "slim") {
		t.Fatal("Dockerfile must use a slim variant of Debian")
	}
}

// TestMaintainerDocsDocumentContainerRecoveryBoundary 锁定双语维护者文档中的
// GHCR 恢复语义：GitHub Release 与 GHCR 非原子，失败必须显式重跑发布 job。
func TestMaintainerDocsDocumentContainerRecoveryBoundary(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	requiredFragments := map[string][]string{
		"docs/en/maintainers/development.md": {
			"If GHCR publication fails",
			"same verified container artifacts",
			"No retry loop",
		},
		"docs/zh-CN/maintainers/development.md": {
			"若 GHCR 发布失败",
			"同一批 verified-container artifact",
			"不使用 retry loop",
		},
	}
	for relativePath, fragments := range requiredFragments {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatalf("read %s: %v", relativePath, err)
		}
		document := string(body)
		for _, fragment := range fragments {
			if !strings.Contains(document, fragment) {
				t.Fatalf("%s must document container recovery boundary with %q", relativePath, fragment)
			}
		}
	}
}
