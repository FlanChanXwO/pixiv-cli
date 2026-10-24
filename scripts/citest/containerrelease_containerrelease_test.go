// Package citest_test 验证容器发布契约：固定 base digest、非 root 运行、
// 不嵌入密钥或本地状态，并且发布只使用受信 release 产物。
package citest_test

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

// TestContainerPublishWorkflowUsesTrustedReleaseArtifacts 锁定两个 registry 共用的
// release_run_id 恢复输入、immutable handoff 校验与 protected release
// Environment 边界。
func TestContainerPublishWorkflowUsesTrustedReleaseArtifacts(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	body, err := os.ReadFile(filepath.Join(root, ".github/workflows/publish-dockerhub.yml"))
	if err != nil {
		t.Fatalf("read Docker Hub publish workflow: %v", err)
	}
	text := string(body)
	for _, fragment := range []string{
		"name: Publish container images",
		"name: Publish pixiv-cli container images",
		"workflow_run:",
		"- Release",
		"- completed",
		"workflow_dispatch:",
		"release_run_id:",
		"release_run_id must be a positive decimal number",
		"environment: release",
		"actions: read",
		"contents: read",
		"packages: write",
		"name: prepared-release-checksums",
		"pattern: verified-container-*",
		"verify-handoff-set",
		"--section container",
		"--run-head-sha",
		"secrets.DOCKER_HUB_TOKEN",
		"docker login docker.io --username",
		"--password-stdin",
		"docker.io/flanchanxwo/pixiv-cli",
		"ghcr.io/flanchanxwo/pixiv-cli",
		"docker login ghcr.io --username",
		"docker manifest create",
		"docker manifest push",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("Docker Hub publish workflow must contain %q", fragment)
		}
	}
	if strings.Contains(text, "docker login docker.io --username \"$DOCKER_HUB_USERNAME\" --password \"$DOCKER_HUB_TOKEN\"") {
		t.Fatal("Docker Hub token must be passed through docker login stdin, not argv")
	}
	releaseBody, err := os.ReadFile(filepath.Join(root, ".github/workflows/release.yml"))
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}
	releaseText := string(releaseBody)
	if strings.Contains(releaseText, "publish_container:") || strings.Contains(releaseText, "docker push \"ghcr.io/flanchanxwo/pixiv-cli") {
		t.Fatal("Release workflow must not publish registry images; the independent container publisher owns registry pushes")
	}
}
