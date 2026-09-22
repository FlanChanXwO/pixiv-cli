package main

import (
	"strings"
	"testing"
)

func TestTestTriggerRequiresFirstNonEmptyLine(t *testing.T) {
	cases := []struct {
		name    string
		comment string
		want    bool
	}{
		{"bare", "/test", true},
		{"leading blank lines", "\n\n  \n/test", true},
		{"trailing content after trigger", "/test\n\nrun the suite", true},
		{"trailing spaces on the trigger line", "   /test   ", true},
		{"argument", "/test abc", false},
		{"suffix", "/test123", false},
		{"prefix text", "please /test", false},
		{"trigger not first non-empty line", "foo\n\n/test", false},
		{"empty", "", false},
		{"whitespace only", "\n   \n", false},
		{"case sensitive", "/TEST", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := testTrigger(tc.comment); got != tc.want {
				t.Fatalf("testTrigger(%q) = %t, want %t", tc.comment, got, tc.want)
			}
		})
	}
}

func TestTestTriggerDoesNotScanForEmbeddedTrigger(t *testing.T) {
	// §5.1 forbids scanning the whole comment for /test.
	comment := "some regular comment\n\n/test\n\nmore text"
	if testTrigger(comment) {
		t.Fatal("testTrigger accepted an embedded /test that is not the first non-empty line")
	}
}

func TestResolveEffectiveFallsBackToPrCommands(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\necho *\n")
	pr := changes + verifySec + fence + "commands\necho from-pr\n" + fence + checklist

	effective, err := resolveEffectiveCommands(pr, "/test", w, "pixiv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if effective.Source != commandSourcePR {
		t.Fatalf("source = %q, want %q", effective.Source, commandSourcePR)
	}
	if len(effective.Commands.Commands) != 1 {
		t.Fatalf("expected 1 command: %#v", effective.Commands)
	}
	if effective.Commands.Commands[0].Display != "echo from-pr" {
		t.Fatalf("unexpected command: %#v", effective.Commands.Commands[0])
	}
	if effective.Hash != hashCommands(effective.Commands) {
		t.Fatal("effective hash must cover the commands actually executed")
	}
}

func TestResolveEffectiveCommentOverrideReplacesPRCommands(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\necho *\n")
	pr := changes + verifySec + fence + "commands\necho from-pr\n" + fence + checklist
	comment := "/test\n\n" + fence + "commands\necho from-comment\n" + fence

	effective, err := resolveEffectiveCommands(pr, comment, w, "pixiv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if effective.Source != commandSourceComment {
		t.Fatalf("source = %q, want %q", effective.Source, commandSourceComment)
	}
	if len(effective.Commands.Commands) != 1 {
		t.Fatalf("expected 1 command: %#v", effective.Commands)
	}
	if effective.Commands.Commands[0].Display != "echo from-comment" {
		t.Fatalf("comment override must fully replace PR commands: %#v", effective.Commands.Commands)
	}
	for _, command := range effective.Commands.Commands {
		if strings.Contains(command.Display, "from-pr") {
			t.Fatal("comment override must not merge PR commands")
		}
	}
}

func TestResolveEffectiveCommentOverrideChangesIdentity(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\necho *\n")
	pr := changes + verifySec + fence + "commands\necho from-pr\n" + fence + checklist

	prEffective, err := resolveEffectiveCommands(pr, "/test", w, "pixiv")
	if err != nil {
		t.Fatalf("resolve pr: %v", err)
	}
	override := "/test\n\n" + fence + "commands\necho from-comment\n" + fence
	commentEffective, err := resolveEffectiveCommands(pr, override, w, "pixiv")
	if err != nil {
		t.Fatalf("resolve comment: %v", err)
	}
	if prEffective.Hash == commentEffective.Hash {
		t.Fatal("same HEAD with different effective commands must yield a different verification hash")
	}

	// Same commands from either source must be the same execution identity: the
	// hash covers the commands, not their origin.
	same := "/test\n\n" + fence + "commands\necho from-pr\n" + fence
	sameEffective, err := resolveEffectiveCommands(pr, same, w, "pixiv")
	if err != nil {
		t.Fatalf("resolve same: %v", err)
	}
	if sameEffective.Hash != prEffective.Hash {
		t.Fatal("identical effective commands must produce the same hash regardless of source")
	}
}

func TestResolveEffectiveInvalidOverrideFailsClosed(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\necho *\n")
	pr := changes + verifySec + fence + "commands\necho from-pr\n" + fence + checklist

	cases := []struct {
		name    string
		comment string
		want    string
	}{
		{
			name:    "empty block",
			comment: "/test\n\n" + fence + "commands\n\n" + fence,
			want:    "The commands block must contain at least one command.",
		},
		{
			name:    "unclosed block",
			comment: "/test\n\n" + fence + "commands\necho x\n",
			want:    "The commands block is not closed.",
		},
		{
			name:    "multiple blocks",
			comment: "/test\n\n" + fence + "commands\necho a\n" + fence + "\n" + fence + "commands\necho b\n" + fence,
			want:    "Only one fenced commands block is allowed.",
		},
		{
			name:    "parse failure",
			comment: "/test\n\n" + fence + "commands\necho a; rm -rf /\n" + fence,
			want:    "Command 1 is invalid: ",
		},
		{
			name:    "whitelist rejection",
			comment: "/test\n\n" + fence + "commands\ncurl https://example.com\n" + fence,
			want:    "Command 1 is not allowed: ",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveEffectiveCommands(pr, tc.comment, w, "pixiv")
			if err == nil {
				t.Fatal("invalid override must fail closed, not fall back to PR commands")
			}
			if !strings.HasPrefix(err.Error(), tc.want) {
				t.Fatalf("error = %q, want prefix %q", err.Error(), tc.want)
			}
		})
	}
}

func TestResolveEffectiveIgnoresNonCommandsFencesInComment(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\necho *\n")
	pr := changes + verifySec + fence + "commands\necho from-pr\n" + fence + checklist
	// A `test` fence is ordinary Markdown, so this is a plain /test with no override.
	comment := "/test\n\n" + fence + "test\necho not-an-override\n" + fence

	effective, err := resolveEffectiveCommands(pr, comment, w, "pixiv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if effective.Source != commandSourcePR {
		t.Fatalf("source = %q, want %q", effective.Source, commandSourcePR)
	}
	if effective.Commands.Commands[0].Display != "echo from-pr" {
		t.Fatalf("non-commands fence must not override: %#v", effective.Commands.Commands)
	}
}

func TestResolveEffectiveWithoutTriggerKeepsPRCommands(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\necho *\n")
	pr := changes + verifySec + fence + "commands\necho from-pr\n" + fence + checklist

	// A comment that merely mentions commands elsewhere is not a trigger; the
	// caller decides triggering, this function only resolves the declaration.
	effective, err := resolveEffectiveCommands(pr, "not a trigger\n"+fence+"commands\necho sneaky\n"+fence, w, "pixiv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if effective.Source != commandSourcePR {
		t.Fatalf("source = %q, want %q", effective.Source, commandSourcePR)
	}
}

func TestResolveEffectiveReportsNeitherSourceWhenPRDeclaresNothing(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\n")
	pr := changes + verifySec + checklist

	effective, err := resolveEffectiveCommands(pr, "/test", w, "pixiv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if effective.Source != commandSourceNone {
		t.Fatalf("source = %q, want %q", effective.Source, commandSourceNone)
	}
	if effective.Hash != "" {
		t.Fatalf("no effective commands must not produce a hash: %q", effective.Hash)
	}
}

func TestResolveEffectiveRejectsInvalidPRDeclaration(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\n")
	pr := changes + verifySec + fence + "commands\ncurl https://example.com\n" + fence + checklist

	_, err := resolveEffectiveCommands(pr, "/test", w, "pixiv")
	if err == nil {
		t.Fatal("invalid PR declaration must not become effective commands")
	}
}

// A comment override replaces the PR declaration outright (§5.3), so a stale or
// invalid PR commands block must not veto an otherwise valid override. The
// override is still validated against the same trusted whitelist, so this does
// not widen what may execute.
func TestResolveEffectiveValidOverrideIgnoresInvalidPRDeclaration(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\necho *\n")
	pr := changes + verifySec + fence + "commands\ncurl https://example.com\n" + fence + checklist
	comment := "/test\n\n" + fence + "commands\necho from-comment\n" + fence

	effective, err := resolveEffectiveCommands(pr, comment, w, "pixiv")
	if err != nil {
		t.Fatalf("valid override must not be vetoed by a stale PR declaration: %v", err)
	}
	if effective.Source != commandSourceComment {
		t.Fatalf("source = %q, want %q", effective.Source, commandSourceComment)
	}
	if len(effective.Commands.Commands) != 1 || effective.Commands.Commands[0].Display != "echo from-comment" {
		t.Fatalf("unexpected effective commands: %#v", effective.Commands.Commands)
	}
}

// A comment override is validated by the same trusted whitelist as the PR
// declaration; it must not be a path to additional privileges (§7).
func TestResolveEffectiveOverrideCannotBypassWhitelist(t *testing.T) {
	w := loadWhitelist(t, "pixiv *\necho *\n")
	pr := changes + verifySec + fence + "commands\necho from-pr\n" + fence + checklist
	comment := "/test\n\n" + fence + "commands\nrm -rf /\n" + fence

	_, err := resolveEffectiveCommands(pr, comment, w, "pixiv")
	if err == nil {
		t.Fatal("override must be rejected when it is not whitelisted")
	}
	if !strings.HasPrefix(err.Error(), "Command 1 is not allowed: ") {
		t.Fatalf("unexpected error: %v", err)
	}
}
