package releasecontract_test

import (
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/releasecontract"
)

func TestPinnedRustToolchainReturnsAuditedReleaseProvenance(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"x86_64-apple-darwin":       "1.96.0",
		"aarch64-apple-darwin":      "1.96.1",
		"x86_64-unknown-linux-gnu":  "1.96.1",
		"aarch64-unknown-linux-gnu": "1.96.1",
		"x86_64-pc-windows-msvc":    "1.96.0",
		"aarch64-pc-windows-msvc":   "1.96.1",
	}
	for target, toolchain := range want {
		got, ok := releasecontract.PinnedRustToolchain(target)
		if !ok || got != toolchain {
			t.Errorf("PinnedRustToolchain(%q) = (%q, %v), want (%q, true)", target, got, ok, toolchain)
		}
	}
	if got, ok := releasecontract.PinnedRustToolchain("unsupported-target"); ok || got != "" {
		t.Fatalf("PinnedRustToolchain(unsupported) = (%q, %v), want (empty, false)", got, ok)
	}
}

func TestFixedTargetsCoverSixReleasePlatforms(t *testing.T) {
	t.Parallel()

	got := releasecontract.FixedTargets()
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

func TestTargetRustTargetMapsGoTriples(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"darwin/amd64":  "x86_64-apple-darwin",
		"darwin/arm64":  "aarch64-apple-darwin",
		"linux/amd64":   "x86_64-unknown-linux-gnu",
		"linux/arm64":   "aarch64-unknown-linux-gnu",
		"windows/amd64": "x86_64-pc-windows-msvc",
		"windows/arm64": "aarch64-pc-windows-msvc",
	}
	for _, target := range releasecontract.FixedTargets() {
		if got := target.RustTarget(); got != want[target.String()] {
			t.Errorf("(%s).RustTarget() = %q, want %q", target.String(), got, want[target.String()])
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
	for _, target := range releasecontract.FixedTargets() {
		got := releasecontract.ArchiveName("0.1.0", target)
		if got != want[target.String()] {
			t.Errorf("ArchiveName(0.1.0, %s) = %q, want %q", target.String(), got, want[target.String()])
		}
	}
}

func TestValidateVersionRejectsLeadingVOrNonSemantic(t *testing.T) {
	t.Parallel()

	for _, version := range []string{"v0.1.0", "1not-semver", "0.1", "0.1.0.1"} {
		if err := releasecontract.ValidateVersion(version); err == nil {
			t.Errorf("ValidateVersion(%q) error = nil, want rejection", version)
		}
	}
	if err := releasecontract.ValidateVersion("0.1.0-beta.1+build-2"); err != nil {
		t.Fatalf("ValidateVersion(semver) error = %v", err)
	}
}

func TestChannelClassifiesStableAndPrerelease(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		version string
		want    string
	}{
		{version: "0.1.0", want: "stable"},
		{version: "0.1.0+build-1", want: "stable"},
		{version: "0.1.0-rc.1", want: "prerelease"},
		{version: "0.1.0-rc.1+build-1", want: "prerelease"},
	} {
		if got := releasecontract.Channel(test.version); got != test.want {
			t.Errorf("Channel(%q) = %q, want %q", test.version, got, test.want)
		}
		if strings.Contains(test.version, "-") {
			if err := releasecontract.ValidateVersion(test.version); err != nil {
				t.Errorf("ValidateVersion(%q) error = %v", test.version, err)
			}
		}
	}
}
