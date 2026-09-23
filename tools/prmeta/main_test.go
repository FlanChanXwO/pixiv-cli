package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/tools/internal/verificationpolicy"
)

const (
	fence     = "```"
	changes   = "## 变更点 / Changes\n\n- change\n\n"
	verifySec = "## 验证步骤 / Verification\n\n"
	checklist = "\n\n## 检查清单 / Checklist\n\n- [x] one\n- [x] two\n"
)

func loadWhitelist(t *testing.T, content string) *verificationpolicy.Whitelist {
	t.Helper()
	path := filepath.Join(t.TempDir(), "command-whitelist.txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := verificationpolicy.LoadWhitelist(path)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestValidateAcceptsCommandsFence(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\npixiv !auth\necho *\n")
	body := changes + verifySec + fence + "commands\necho ABC-123 | pixiv detail\n" + fence + checklist
	got := validate(body, w, "pixiv", t.TempDir())
	if !got.TemplateOK || !got.CommandsOK {
		t.Fatalf("expected template and commands success: %#v", got)
	}
	if len(got.Commands.Commands) != 1 || got.Hash == "" {
		t.Fatalf("expected one hashed command: %#v", got)
	}
	if got.CommandsDesc != "1 default verification command accepted." {
		t.Fatalf("unexpected commands description: %q", got.CommandsDesc)
	}
	if got.TemplateDesc != "Required PR sections and checklist are complete." {
		t.Fatalf("unexpected template description: %q", got.TemplateDesc)
	}
}

func TestValidateIgnoresLegacyAndOtherFences(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\n")
	for _, lang := range []string{"test", "bash", "sh", "text", "json", ""} {
		body := changes + verifySec + fence + lang + "\npixiv --version\n" + fence + checklist
		got := validate(body, w, "pixiv", t.TempDir())
		if !got.TemplateOK {
			t.Fatalf("lang %q: expected template success: %#v", lang, got)
		}
		if !got.CommandsOK {
			t.Fatalf("lang %q: %q must be ignored as ordinary markdown: %#v", lang, lang, got)
		}
		if len(got.Commands.Commands) != 0 {
			t.Fatalf("lang %q: expected no declared commands: %#v", lang, got)
		}
		if got.CommandsDesc != "No default verification commands declared." {
			t.Fatalf("lang %q: unexpected description %q", lang, got.CommandsDesc)
		}
	}
}

func TestValidateCountsMultipleCommands(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\necho *\n")
	body := changes + verifySec + fence + "commands\necho one\npixiv --version\necho three\n" + fence + checklist
	got := validate(body, w, "pixiv", t.TempDir())
	if !got.CommandsOK || len(got.Commands.Commands) != 3 {
		t.Fatalf("expected three commands: %#v", got)
	}
	if got.CommandsDesc != "3 default verification commands accepted." {
		t.Fatalf("unexpected description: %q", got.CommandsDesc)
	}
}

func TestValidateRejectsMultipleCommandsBlocks(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\n")
	body := changes + verifySec + fence + "commands\npixiv --version\n" + fence + "\n" +
		fence + "commands\npixiv --version\n" + fence + checklist
	got := validate(body, w, "pixiv", t.TempDir())
	if got.CommandsOK {
		t.Fatalf("expected failure for multiple commands blocks: %#v", got)
	}
	if got.CommandsDesc != "Only one fenced commands block is allowed." {
		t.Fatalf("unexpected description: %q", got.CommandsDesc)
	}
}

func TestValidateRejectsEmptyCommandsBlock(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\n")
	body := changes + verifySec + fence + "commands\n\n\n" + fence + checklist
	got := validate(body, w, "pixiv", t.TempDir())
	if got.CommandsOK {
		t.Fatalf("expected failure for empty commands block: %#v", got)
	}
	if got.CommandsDesc != "The commands block must contain at least one command." {
		t.Fatalf("unexpected description: %q", got.CommandsDesc)
	}
}

func TestValidateRejectsUnclosedCommandsBlock(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\n")
	body := changes + verifySec + fence + "commands\npixiv --version\n" + checklist
	got := validate(body, w, "pixiv", t.TempDir())
	if got.CommandsOK {
		t.Fatalf("expected failure for unclosed commands block: %#v", got)
	}
	if got.CommandsDesc != "The commands block is not closed." {
		t.Fatalf("unexpected description: %q", got.CommandsDesc)
	}
}

func TestValidateReportsInvalidCommandByIndex(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\necho *\n")
	body := changes + verifySec + fence + "commands\necho ok\npixiv --version; rm -rf /\n" + fence + checklist
	got := validate(body, w, "pixiv", t.TempDir())
	if got.CommandsOK {
		t.Fatalf("expected failure for invalid command: %#v", got)
	}
	if !strings.HasPrefix(got.CommandsDesc, "Command 2 is invalid: ") {
		t.Fatalf("unexpected description: %q", got.CommandsDesc)
	}
}

func TestValidateReportsDisallowedCommandByIndex(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\necho *\n")
	body := changes + verifySec + fence + "commands\necho ok\ncurl https://example.com\n" + fence + checklist
	got := validate(body, w, "pixiv", t.TempDir())
	if got.CommandsOK {
		t.Fatalf("expected failure for disallowed command: %#v", got)
	}
	if !strings.HasPrefix(got.CommandsDesc, "Command 2 is not allowed: ") {
		t.Fatalf("unexpected description: %q", got.CommandsDesc)
	}
}

func TestValidateTemplateMessages(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\n")
	missing := validate(changes+"## 检查清单 / Checklist\n\n- [x] one\n", w, "pixiv", t.TempDir())
	if missing.TemplateOK {
		t.Fatalf("expected missing-section failure: %#v", missing)
	}
	if missing.TemplateDesc != "Missing required PR section(s): Verification." {
		t.Fatalf("unexpected description: %q", missing.TemplateDesc)
	}
	unchecked := validate(changes+verifySec+"## 检查清单 / Checklist\n\n- [ ] one\n", w, "pixiv", t.TempDir())
	if unchecked.TemplateOK {
		t.Fatalf("expected checklist failure: %#v", unchecked)
	}
	if unchecked.TemplateDesc != "Complete all required checklist items." {
		t.Fatalf("unexpected description: %q", unchecked.TemplateDesc)
	}
}

func TestRepositoryPRTemplateMatchesValidator(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\n")
	template, err := os.ReadFile(filepath.Join("..", "..", ".github", "PULL_REQUEST_TEMPLATE.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ReplaceAll(string(template), "- [ ]", "- [x]")
	got := validate(body, w, "pixiv", t.TempDir())
	if !got.TemplateOK {
		t.Fatalf("repository PR template is incompatible with validator: %s", got.TemplateDesc)
	}
}
