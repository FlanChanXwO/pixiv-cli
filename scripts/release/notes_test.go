package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateAcceptsMatchingBilingualReleaseNotes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeReleaseNote(t, root, "en.md", `# v1.2.0 — 2026-07-30

## Added

- Added APNG output. ([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))

**Full Changelog**: [v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)
`)
	writeReleaseNote(t, root, "zh-CN.md", `# v1.2.0 — 2026-07-30

## 新增

- 新增 APNG 输出。([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))

**完整变更**：[v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)
`)

	if err := validateReleaseDirectory(root, "1.2.0", "v1.1.0", false); err != nil {
		t.Fatalf("validate release directory: %v", err)
	}
}

func TestValidateRejectsMismatchedBilingualSources(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeReleaseNote(t, root, "en.md", `# v1.2.0 — 2026-07-30

## Changed

- Changed output. ([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))

**Full Changelog**: [v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)
`)
	writeReleaseNote(t, root, "zh-CN.md", `# v1.2.0 — 2026-07-30

## 变更

- 修改输出。([#43](https://github.com/FlanChanXwO/pixiv-cli/pull/43))

**完整变更**：[v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)
`)

	err := validateReleaseDirectory(root, "1.2.0", "v1.1.0", false)
	if err == nil || !strings.Contains(err.Error(), "source sets differ") {
		t.Fatalf("validate error = %v, want bilingual source mismatch", err)
	}
}

func TestSyncHistoryDryRunAndApplyPreservesAssets(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	writeReleaseNote(t, directory, "en.md", "# v1.1.0 — 2026-07-30\n\n## Added\n\n- Added one. ([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))\n\n**Full Changelog**: [v1.0.0...v1.1.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.0...v1.1.0)\n")
	writeReleaseNote(t, directory, "zh-CN.md", "# v1.1.0 — 2026-07-30\n\n## 新增\n\n- 新增一。([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))\n\n**完整变更**：[v1.0.0...v1.1.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.0...v1.1.0)\n")
	var releaseBody string
	var patchCount int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/repos/owner/project/releases/tags/v1.1.0":
			_ = json.NewEncoder(response).Encode(githubRelease{ID: 99, TagName: "v1.1.0", Body: releaseBody, Assets: []githubReleaseAsset{{Name: "pixiv-cli.tar.gz"}}})
		case request.Method == http.MethodPatch && request.URL.Path == "/repos/owner/project/releases/99":
			patchCount++
			var payload githubReleaseWrite
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatalf("decode patch: %v", err)
			}
			releaseBody = payload.Body
			response.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(response).Encode(githubRelease{ID: 99, TagName: "v1.1.0", Body: releaseBody, Assets: []githubReleaseAsset{{Name: "pixiv-cli.tar.gz"}}})
		default:
			t.Fatalf("unexpected %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	config := syncHistoryConfig{Repository: "owner/project", Version: "1.1.0", Directory: directory, Client: githubClient{baseURL: server.URL, client: server.Client()}}
	if err := syncHistoricalRelease(context.Background(), config); err != nil {
		t.Fatalf("dry-run sync: %v", err)
	}
	if patchCount != 0 {
		t.Fatalf("dry-run patched %d times", patchCount)
	}
	config.Apply = true
	if err := syncHistoricalRelease(context.Background(), config); err != nil {
		t.Fatalf("apply sync: %v", err)
	}
	if patchCount != 1 || !strings.Contains(releaseBody, "# English") || !strings.Contains(releaseBody, "# 简体中文") {
		t.Fatalf("sync result patchCount=%d body=%q", patchCount, releaseBody)
	}
}

func writeReleaseNote(t *testing.T, root, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestValidateRejectsMaintenanceSection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeReleaseNote(t, root, "en.md", `# v1.2.0 — 2026-07-30

## Maintenance

- Refactored the transport. ([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))

**Full Changelog**: [v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)
`)
	writeReleaseNote(t, root, "zh-CN.md", `# v1.2.0 — 2026-07-30

## 维护

- 重构传输层。([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))

**完整变更**：[v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)
`)

	err := validateReleaseDirectory(root, "1.2.0", "v1.1.0", false)
	if err == nil || !strings.Contains(err.Error(), "unsupported section") {
		t.Fatalf("validate error = %v, want unsupported section", err)
	}
}

func TestValidateRejectsUnpairedBilingualEntries(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeReleaseNote(t, root, "en.md", `# v1.2.0 — 2026-07-30

## Added

- Added one. ([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))
- Added two. ([#43](https://github.com/FlanChanXwO/pixiv-cli/pull/43))

**Full Changelog**: [v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)
`)
	writeReleaseNote(t, root, "zh-CN.md", `# v1.2.0 — 2026-07-30

## 新增

- 新增一、二。([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42), [#43](https://github.com/FlanChanXwO/pixiv-cli/pull/43))

**完整变更**：[v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)
`)

	err := validateReleaseDirectory(root, "1.2.0", "v1.1.0", false)
	if err == nil || !strings.Contains(err.Error(), "entry count differs") {
		t.Fatalf("validate error = %v, want bilingual entry count mismatch", err)
	}
}

func TestValidateSourceCoverageAcceptsInternalMarker(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeReleaseNote(t, root, "en.md", `# v1.2.0 — 2026-07-30

## Fixed

- Fixed a visible crash. ([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))

<!-- internal: [#43](https://github.com/FlanChanXwO/pixiv-cli/pull/43), [a1b2c3d](https://github.com/FlanChanXwO/pixiv-cli/commit/a1b2c3d9f8e7d6c5b4a39281706f5e4d3c2b1a09) -->

**Full Changelog**: [v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)
`)
	writeReleaseNote(t, root, "zh-CN.md", `# v1.2.0 — 2026-07-30

## 修复

- 修复可见崩溃。([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))

<!-- internal: [#43](https://github.com/FlanChanXwO/pixiv-cli/pull/43), [a1b2c3d](https://github.com/FlanChanXwO/pixiv-cli/commit/a1b2c3d9f8e7d6c5b4a39281706f5e4d3c2b1a09) -->

**完整变更**：[v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)
`)
	report := auditReport{Sources: []auditSource{
		{URL: "https://github.com/FlanChanXwO/pixiv-cli/pull/42"},
		{URL: "https://github.com/FlanChanXwO/pixiv-cli/pull/43"},
		{URL: "https://github.com/FlanChanXwO/pixiv-cli/commit/a1b2c3d9f8e7d6c5b4a39281706f5e4d3c2b1a09"},
	}}

	if err := validateSourceCoverage(root, "1.2.0", "v1.1.0", false, report); err != nil {
		t.Fatalf("validate source coverage: %v", err)
	}
	body, err := NotesFromChangelog(filepath.Join(root, "en.md"), "1.2.0")
	if err != nil {
		t.Fatalf("render notes: %v", err)
	}
	if strings.Contains(string(body), "internal:") {
		t.Fatalf("rendered notes contain the internal marker: %q", body)
	}
}

func TestValidateRejectsInternalMarkerText(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeReleaseNote(t, root, "en.md", `# v1.2.0 — 2026-07-30

## Fixed

- Fixed a visible crash. ([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))

<!-- internal: refactor only -->

**Full Changelog**: [v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)
`)
	writeReleaseNote(t, root, "zh-CN.md", `# v1.2.0 — 2026-07-30

## 修复

- 修复可见崩溃。([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))

**完整变更**：[v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)
`)

	if err := validateReleaseDirectory(root, "1.2.0", "v1.1.0", false); err == nil || !strings.Contains(err.Error(), "internal marker") {
		t.Fatalf("validate error = %v, want internal marker rejection", err)
	}
}

func TestValidateAcceptsUnreleasedWorkspace(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeReleaseNote(t, root, "en.md", `# Unreleased

## Added

- Added one. ([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))

**Full Changelog**: _Added when the next version is tagged._

<!-- internal:
  [#43](https://github.com/FlanChanXwO/pixiv-cli/pull/43)
-->
`)
	writeReleaseNote(t, root, "zh-CN.md", `# 未发布

## 新增

- 新增一。([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))

**完整变更**：打下一版 tag 时补充。

<!-- internal:
  [#43](https://github.com/FlanChanXwO/pixiv-cli/pull/43)
-->
`)
	report := auditReport{Sources: []auditSource{
		{URL: "https://github.com/FlanChanXwO/pixiv-cli/pull/42"},
		{URL: "https://github.com/FlanChanXwO/pixiv-cli/pull/43"},
	}}

	if err := validateSourceCoverage(root, "", "", true, report); err != nil {
		t.Fatalf("validate unreleased workspace: %v", err)
	}
}

func TestValidateRejectsMispairedBilingualSections(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeReleaseNote(t, root, "en.md", `# v1.2.0 — 2026-07-30

## Added

- Added one. ([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))

## Fixed

- Fixed one. ([#43](https://github.com/FlanChanXwO/pixiv-cli/pull/43))

**Full Changelog**: [v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)
`)
	writeReleaseNote(t, root, "zh-CN.md", `# v1.2.0 — 2026-07-30

## 变更

- 修改一。([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))

## 修复

- 修复一。([#43](https://github.com/FlanChanXwO/pixiv-cli/pull/43))

**完整变更**：[v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)
`)

	if err := validateReleaseDirectory(root, "1.2.0", "v1.1.0", false); err == nil || !strings.Contains(err.Error(), "is paired with") {
		t.Fatalf("validate error = %v, want mispaired section rejection", err)
	}
}

func TestValidateAcceptsInternalOnlyRelease(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeReleaseNote(t, root, "en.md", `# v1.2.0 — 2026-07-30

**Full Changelog**: [v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)

<!-- internal:
  [#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42)
-->
`)
	writeReleaseNote(t, root, "zh-CN.md", `# v1.2.0 — 2026-07-30

**完整变更**：[v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)

<!-- internal:
  [#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42)
-->
`)
	report := auditReport{Sources: []auditSource{{URL: "https://github.com/FlanChanXwO/pixiv-cli/pull/42"}}}

	if err := validateSourceCoverage(root, "1.2.0", "v1.1.0", false, report); err != nil {
		t.Fatalf("validate internal-only release: %v", err)
	}
}
