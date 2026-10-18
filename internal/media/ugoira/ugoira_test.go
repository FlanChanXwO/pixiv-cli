package ugoira_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCGODisabledBuild(t *testing.T) {
	command := exec.Command("go", "build", "-o", filepath.Join(t.TempDir(), "pixiv"), "./cmd/pixiv")
	command.Dir = filepath.Clean(filepath.Join("..", "..", ".."))
	command.Env = append(os.Environ(), "CGO_ENABLED=0")
	if body, err := command.CombinedOutput(); err != nil {
		t.Fatalf("CGO_ENABLED=0 go build: %v\n%s", err, body)
	}
}

// TestReleaseLicenseOutputsUseLFCheckout 锁定发布归档的许可证输入在 Windows checkout 也保留 LF；
// 否则 runner record 虽自洽，跨平台 consolidation 仍会因 archive member bytes 不一致而拒绝证据。
func TestReleaseLicenseOutputsUseLFCheckout(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	for _, source := range []string{
		"LICENSE",
		"THIRD_PARTY_LICENSES.md",
		"third_party/licenses/Go-1.26.3/LICENSE",
	} {
		command := exec.Command("git", "-C", repoRoot, "check-attr", "text", "eol", "--", source)
		body, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("read Git line-ending attributes for %q: %v\n%s", source, err, body)
		}
		output := string(body)
		if !strings.Contains(output, "text: set") || !strings.Contains(output, "eol: lf") {
			t.Fatalf("release license input %q must use LF checkout, got %q", source, strings.TrimSpace(output))
		}
	}
}
