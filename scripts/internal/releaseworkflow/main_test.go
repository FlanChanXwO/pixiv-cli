package releaseworkflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckWorkflowRequiresRustFormatGate(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join(findRepositoryRoot(t), ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}
	if err := checkWorkflow(body); err != nil {
		t.Fatalf("release workflow policy rejected checked-in workflow: %v", err)
	}
}

func TestCheckPinnedGitHubKnownHosts(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join(findRepositoryRoot(t), "templates", "homebrew", "github.com-known-hosts"))
	if err != nil {
		t.Fatalf("read pinned GitHub known_hosts: %v", err)
	}
	if err := checkPinnedGitHubKnownHosts(body); err != nil {
		t.Fatalf("checked-in GitHub known_hosts rejected: %v", err)
	}
	// 先归一化再构造 CRLF，避免 Windows 已经以 CRLF 检出的 fixture
	// 再替换换行时形成 \r\r\n，确保这里覆盖真实的 Windows 输入。
	crlfBody := []byte(strings.ReplaceAll(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n", "\r\n"))
	if err := checkPinnedGitHubKnownHosts(crlfBody); err != nil {
		t.Fatalf("CRLF checked-out GitHub known_hosts rejected: %v", err)
	}
	for _, mutation := range [][]byte{
		[]byte("github.com ssh-ed25519 attacker\n"),
		append(append([]byte(nil), body...), []byte("github.com ssh-rsa extra\n")...),
		append(append([]byte(nil), crlfBody...), []byte("github.com ssh-rsa extra\r\n")...),
	} {
		if err := checkPinnedGitHubKnownHosts(mutation); err == nil {
			t.Fatal("mutated GitHub known_hosts fixture was accepted")
		}
	}
}
