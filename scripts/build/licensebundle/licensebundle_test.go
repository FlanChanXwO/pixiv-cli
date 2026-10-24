package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeLicenseTextPreservesTextWithCanonicalLF(t *testing.T) {
	got := normalizeLicenseText([]byte("first line\r\nsecond line\n\n\n"))
	if want := "first line\nsecond line\n"; string(got) != want {
		t.Fatalf("normalizeLicenseText = %q, want %q", got, want)
	}
}

func TestGenerateRejectsOutputPathsOutsideRepositoryBeforeGo(t *testing.T) {
	base := t.TempDir()
	repository := filepath.Join(base, "repository")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outside, "licenses"), 0o755); err != nil {
		t.Fatal(err)
	}
	outsideIndex := filepath.Join(outside, "THIRD_PARTY_LICENSES.md")
	if err := os.WriteFile(outsideIndex, []byte("outside index\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outsideLicense := filepath.Join(outside, "licenses", "keep")
	if err := os.WriteFile(outsideLicense, []byte("outside license\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repository, "outside-link")); err != nil {
		t.Skipf("test environment cannot create a symlink: %v", err)
	}
	for name, opts := range map[string]options{
		"absolute index": {
			Repository: repository,
			Index:      outsideIndex,
			Licenses:   "third_party/licenses",
			Go:         "missing-go-must-not-run",
		},
		"escaped licenses": {
			Repository: repository,
			Index:      "THIRD_PARTY_LICENSES.md",
			Licenses:   "../outside/licenses",
			Go:         "missing-go-must-not-run",
		},
		"lexical traversal": {
			Repository: repository,
			Index:      "generated/../THIRD_PARTY_LICENSES.md",
			Licenses:   "third_party/licenses",
			Go:         "missing-go-must-not-run",
		},
		"overlapping outputs": {
			Repository: repository,
			Index:      "third_party/licenses/index.md",
			Licenses:   "third_party/licenses",
			Go:         "missing-go-must-not-run",
		},
		"symlinked index": {
			Repository: repository,
			Index:      "outside-link/THIRD_PARTY_LICENSES.md",
			Licenses:   "third_party/licenses",
			Go:         "missing-go-must-not-run",
		},
		"symlinked licenses": {
			Repository: repository,
			Index:      "THIRD_PARTY_LICENSES.md",
			Licenses:   "outside-link/licenses",
			Go:         "missing-go-must-not-run",
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := generate(opts)
			if err == nil || (!strings.Contains(err.Error(), "repository-relative") && !strings.Contains(err.Error(), "must not overlap")) {
				t.Fatalf("generate error = %v, want repository-contained output path rejection", err)
			}
			if body, err := os.ReadFile(outsideIndex); err != nil || string(body) != "outside index\n" {
				t.Fatalf("outside index changed: body=%q err=%v", body, err)
			}
			if body, err := os.ReadFile(outsideLicense); err != nil || string(body) != "outside license\n" {
				t.Fatalf("outside license sentinel changed: body=%q err=%v", body, err)
			}
		})
	}
}
