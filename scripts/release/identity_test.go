package main

import (
	"testing"
)

func TestFixedTargetsCoverSixReleasePlatforms(t *testing.T) {
	t.Parallel()

	got := FixedTargets()
	if len(got) != 6 {
		t.Fatalf("FixedTargets() = %d targets, want 6", len(got))
	}
	want := map[string]bool{
		"darwin/amd64":  true,
		"darwin/arm64":  true,
		"linux/amd64":   true,
		"linux/arm64":   true,
		"windows/amd64": true,
		"windows/arm64": true,
	}
	for _, target := range got {
		if !want[target.String()] {
			t.Errorf("FixedTargets() contains unexpected target %q", target.String())
		}
	}
}

func TestArchiveNameMatchesFixedTargetIdentity(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"darwin/amd64":  "pixiv-cli_0.1.0_darwin_amd64.tar.gz",
		"darwin/arm64":  "pixiv-cli_0.1.0_darwin_arm64.tar.gz",
		"linux/amd64":   "pixiv-cli_0.1.0_linux_amd64.tar.gz",
		"linux/arm64":   "pixiv-cli_0.1.0_linux_arm64.tar.gz",
		"windows/amd64": "pixiv-cli_0.1.0_windows_amd64.zip",
		"windows/arm64": "pixiv-cli_0.1.0_windows_arm64.zip",
	}
	for _, target := range FixedTargets() {
		got := ArchiveName("0.1.0", target)
		if got != want[target.String()] {
			t.Errorf("ArchiveName(0.1.0, %s) = %q, want %q", target.String(), got, want[target.String()])
		}
	}
}
