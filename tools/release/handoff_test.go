package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// appendPlatformArtifacts 生成一个最小可用的 release 产物集，用于验证 handoff
// 与校验和绑定，不依赖真实构建。
func appendPlatformArtifacts(t *testing.T, dir, version string, names ...string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("payload:"+name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func sha256File(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func TestReleaseHandoffBindsIdentityAndChecksums(t *testing.T) {
	version := "1.2.3"
	dist := t.TempDir()
	containers := t.TempDir()
	appendPlatformArtifacts(t, dist, version,
		"pixiv-cli_1.2.3_linux_amd64.tar.gz",
		"pixiv-cli_1.2.3_darwin_arm64.tar.gz",
	)
	appendPlatformArtifacts(t, containers, version, "pixiv-cli-linux-amd64.tar")

	output := filepath.Join(t.TempDir(), "release-handoff.json")
	handoff, err := writeReleaseHandoff(releaseHandoffInput{
		Repository:   "FlanChanXwO/pixiv-cli",
		RunID:        4242,
		RunAttempt:   1,
		Workflow:     "Release",
		Tag:          "v" + version,
		CommitSHA:    strings.Repeat("a", 40),
		RunHeadSHA:   strings.Repeat("a", 40),
		Version:      version,
		DistDir:      dist,
		ContainerDir: containers,
		Output:       output,
	})
	if err != nil {
		t.Fatalf("write handoff: %v", err)
	}

	// §15：handoff 必须承载 repository / release_run_id / tag / commit_sha /
	// artifact names / checksums，且校验和必须与磁盘字节一致。
	if handoff.Repository != "FlanChanXwO/pixiv-cli" {
		t.Fatalf("repository = %q", handoff.Repository)
	}
	if handoff.ReleaseRunID != 4242 {
		t.Fatalf("release_run_id = %d", handoff.ReleaseRunID)
	}
	if handoff.Tag != "v"+version || handoff.CommitSHA != strings.Repeat("a", 40) {
		t.Fatalf("tag/commit = %q/%q", handoff.Tag, handoff.CommitSHA)
	}
	if handoff.Workflow != "Release" {
		t.Fatalf("workflow = %q", handoff.Workflow)
	}
	if len(handoff.ProductionArtifacts) != 2 || len(handoff.ContainerArtifacts) != 1 {
		t.Fatalf("artifact sets = %d production, %d container", len(handoff.ProductionArtifacts), len(handoff.ContainerArtifacts))
	}
	for _, artifact := range append(append([]handoffArtifact{}, handoff.ProductionArtifacts...), handoff.ContainerArtifacts...) {
		var dir string
		if strings.HasPrefix(artifact.Name, "pixiv-cli-") {
			dir = containers
		} else {
			dir = dist
		}
		if artifact.SHA256 != sha256File(t, filepath.Join(dir, artifact.Name)) {
			t.Fatalf("artifact %s checksum mismatch: %s", artifact.Name, artifact.SHA256)
		}
		if artifact.Size <= 0 {
			t.Fatalf("artifact %s missing size", artifact.Name)
		}
	}

	// 落盘内容必须可被同一套 verifier 重新读回并校验通过。
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var decoded releaseHandoff
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode handoff: %v", err)
	}
	if err := verifyHandoffArtifacts(decoded, dist, containers); err != nil {
		t.Fatalf("verify handoff artifacts: %v", err)
	}
}

// 篡改产物字节后，handoff 校验必须失败——这正是 publisher 复用 handoff 的意义。
func TestVerifyHandoffArtifactsRejectsTamperedBytes(t *testing.T) {
	version := "1.2.3"
	dist := t.TempDir()
	containers := t.TempDir()
	appendPlatformArtifacts(t, dist, version, "pixiv-cli_1.2.3_linux_amd64.tar.gz")
	appendPlatformArtifacts(t, containers, version, "pixiv-cli-linux-amd64.tar")

	handoff, err := writeReleaseHandoff(releaseHandoffInput{
		Repository:   "FlanChanXwO/pixiv-cli",
		RunID:        7,
		Workflow:     "Release",
		Tag:          "v" + version,
		CommitSHA:    strings.Repeat("b", 40),
		RunHeadSHA:   strings.Repeat("b", 40),
		Version:      version,
		DistDir:      dist,
		ContainerDir: containers,
		Output:       filepath.Join(t.TempDir(), "handoff.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyHandoffArtifacts(handoff, dist, containers); err != nil {
		t.Fatalf("baseline verify: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dist, "pixiv-cli_1.2.3_linux_amd64.tar.gz"), []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyHandoffArtifacts(handoff, dist, containers); err == nil {
		t.Fatal("tampered production artifact must fail handoff verification")
	}
}

// 缺失产物、多出产物、以及身份字段不完整都必须被拒绝。
func TestVerifyHandoffArtifactsRejectsIncompleteSet(t *testing.T) {
	version := "1.2.3"
	dist := t.TempDir()
	containers := t.TempDir()
	appendPlatformArtifacts(t, dist, version, "pixiv-cli_1.2.3_linux_amd64.tar.gz")
	appendPlatformArtifacts(t, containers, version, "pixiv-cli-linux-amd64.tar")

	handoff, err := writeReleaseHandoff(releaseHandoffInput{
		Repository:   "FlanChanXwO/pixiv-cli",
		RunID:        9,
		Workflow:     "Release",
		Tag:          "v" + version,
		CommitSHA:    strings.Repeat("c", 40),
		RunHeadSHA:   strings.Repeat("c", 40),
		Version:      version,
		DistDir:      dist,
		ContainerDir: containers,
		Output:       filepath.Join(t.TempDir(), "handoff.json"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(containers, "pixiv-cli-linux-amd64.tar")); err != nil {
		t.Fatal(err)
	}
	if err := verifyHandoffArtifacts(handoff, dist, containers); err == nil {
		t.Fatal("missing container artifact must fail handoff verification")
	}

	// 身份字段不完整（缺 repository / run id / commit）不能被当作可信 handoff。
	for name, mutate := range map[string]func(*releaseHandoff){
		"repository": func(h *releaseHandoff) { h.Repository = "" },
		"run id":     func(h *releaseHandoff) { h.ReleaseRunID = 0 },
		"tag":        func(h *releaseHandoff) { h.Tag = "" },
		"commit":     func(h *releaseHandoff) { h.CommitSHA = "" },
		"run head":   func(h *releaseHandoff) { h.RunHeadSHA = "" },
		"workflow":   func(h *releaseHandoff) { h.Workflow = "" },
	} {
		broken := handoff
		mutate(&broken)
		if err := verifyHandoffIdentity(broken); err == nil {
			t.Fatalf("incomplete handoff identity (%s) must be rejected", name)
		}
	}
}

// ClawHub 只发布 skill（不发布 release 归档或镜像），因此它需要一个只校验
// handoff 身份、不要求本地已下载产物的模式；否则 publisher 要么跳过校验，
// 要么被迫下载它根本不需要的产物（§15/§19）。
func TestVerifyHandoffIdentityOnly(t *testing.T) {
	handoff := releaseHandoff{
		Repository:   "FlanChanXwO/pixiv-cli",
		ReleaseRunID: 12,
		Workflow:     "Release",
		Tag:          "v1.2.3",
		CommitSHA:    strings.Repeat("d", 40),
		RunHeadSHA:   strings.Repeat("d", 40),
		Version:      "1.2.3",
	}
	if err := verifyHandoffIdentityOnly(handoff, 12, "FlanChanXwO/pixiv-cli", "v1.2.3", strings.Repeat("d", 40)); err != nil {
		t.Fatalf("identity-only verify: %v", err)
	}
	// 身份或 run 绑定不符必须失败，这样才能证明 handoff 来自被解析的那个 run。
	for name, call := range map[string]func() error{
		"run id": func() error {
			return verifyHandoffIdentityOnly(handoff, 13, "FlanChanXwO/pixiv-cli", "v1.2.3", strings.Repeat("d", 40))
		},
		"repo": func() error {
			return verifyHandoffIdentityOnly(handoff, 12, "other/repo", "v1.2.3", strings.Repeat("d", 40))
		},
		"tag": func() error {
			return verifyHandoffIdentityOnly(handoff, 12, "FlanChanXwO/pixiv-cli", "v9.9.9", strings.Repeat("d", 40))
		},
		"run head": func() error {
			return verifyHandoffIdentityOnly(handoff, 12, "FlanChanXwO/pixiv-cli", "v1.2.3", strings.Repeat("e", 40))
		},
	} {
		if err := call(); err == nil {
			t.Fatalf("mismatched %s must be rejected", name)
		}
	}
}
