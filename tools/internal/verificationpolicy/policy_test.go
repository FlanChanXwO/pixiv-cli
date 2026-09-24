package verificationpolicy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLinePreservesQuotedPipeAndSplitsPipeline(t *testing.T) {
	command, err := ParseLine(`echo "a | b" | pixiv detail ABC-123`)
	if err != nil {
		t.Fatal(err)
	}
	if len(command.Stages) != 2 {
		t.Fatalf("stages = %d, want 2", len(command.Stages))
	}
	if got := command.Stages[0].Argv[1]; got != "a | b" {
		t.Fatalf("quoted pipe = %q", got)
	}
	if _, err := ParseLine("pixiv search ABC-123 && echo nope"); err == nil {
		t.Fatal("expected shell operator rejection")
	}
}

func TestWhitelistDenyAndWorkspaceBoundary(t *testing.T) {
	root := t.TempDir()
	whitelistPath := filepath.Join(root, "whitelist.txt")
	if err := os.WriteFile(whitelistPath, []byte("pixiv *\npixiv !auth\ncat @\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "image.jpg"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := LoadWhitelist(whitelistPath)
	if err != nil {
		t.Fatal(err)
	}
	allowed, _ := ParseLine("cat image.jpg | pixiv search")
	if err := w.Validate(allowed, "pixiv", root); err != nil {
		t.Fatalf("allowed pipeline rejected: %v", err)
	}
	denied, _ := ParseLine("pixiv auth list")
	if err := w.Validate(denied, "pixiv", root); err == nil {
		t.Fatal("expected auth rejection")
	}
	proxy, _ := ParseLine("pixiv search foo --proxy=https://example.invalid")
	if err := w.Validate(proxy, "pixiv", root); err == nil {
		t.Fatal("expected inline proxy rejection")
	}
	escape, _ := ParseLine("cat ../outside")
	if err := w.Validate(escape, "pixiv", root); err == nil {
		t.Fatal("expected workspace escape rejection")
	}
}
