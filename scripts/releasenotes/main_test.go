package main

import (
	"context"
	"encoding/json"
	"fmt"
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

	if err := validateReleaseDirectory(root, "1.2.0", "v1.1.0"); err != nil {
		t.Fatalf("validate release directory: %v", err)
	}
}

func TestValidateRejectsMissingEntrySource(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeReleaseNote(t, root, "en.md", `# v1.2.0 — 2026-07-30

## Fixed

- Fixed an issue without an attribution.

**Full Changelog**: [v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)
`)
	writeReleaseNote(t, root, "zh-CN.md", `# v1.2.0 — 2026-07-30

## 修复

- 修复未标注来源的问题。

**完整变更**：[v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)
`)

	err := validateReleaseDirectory(root, "1.2.0", "v1.1.0")
	if err == nil || !strings.Contains(err.Error(), "source") {
		t.Fatalf("validate error = %v, want missing source error", err)
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

	err := validateReleaseDirectory(root, "1.2.0", "v1.1.0")
	if err == nil || !strings.Contains(err.Error(), "source sets differ") {
		t.Fatalf("validate error = %v, want bilingual source mismatch", err)
	}
}

func TestValidateAcceptsInitialReleaseCommitLink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	entry := "([`abcdef0`](https://github.com/FlanChanXwO/pixiv-cli/commit/abcdef0123456789))"
	writeReleaseNote(t, root, "en.md", "# v1.0.0 — 2026-07-30\n\n## Added\n\n- First release. "+entry+"\n\n**Full Changelog**: [v1.0.0 commits](https://github.com/FlanChanXwO/pixiv-cli/commits/v1.0.0)\n")
	writeReleaseNote(t, root, "zh-CN.md", "# v1.0.0 — 2026-07-30\n\n## 新增\n\n- 首次发布。"+entry+"\n\n**完整变更**：[v1.0.0 commits](https://github.com/FlanChanXwO/pixiv-cli/commits/v1.0.0)\n")

	if err := validateReleaseDirectory(root, "1.0.0", ""); err != nil {
		t.Fatalf("validate initial release: %v", err)
	}
}

func TestParseReleaseNoteDeclaration(t *testing.T) {
	t.Parallel()

	note, err := parseReleaseNoteDeclaration(`
## Summary

Implemented a user-visible change.

<!-- release-note
category: Changed
breaking: true
summary: Reworked the download request contract.
none_reason:
-->
`)
	if err != nil {
		t.Fatalf("parse declaration: %v", err)
	}
	if note.Category != "Changed" || !note.Breaking || note.Summary != "Reworked the download request contract." {
		t.Fatalf("release note = %#v", note)
	}
}

func TestParseReleaseNoteDeclarationRequiresNoneReason(t *testing.T) {
	t.Parallel()

	_, err := parseReleaseNoteDeclaration(`<!-- release-note
category: None
breaking: false
summary: No release entry.
none_reason:
-->`)
	if err == nil || !strings.Contains(err.Error(), "none_reason") {
		t.Fatalf("parse error = %v, want none_reason validation", err)
	}
}

func TestRecommendedVersionBump(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		previous string
		notes    []releaseNote
		want     string
	}{
		{name: "maintenance", previous: "v1.0.0", notes: []releaseNote{{Category: "Maintenance", Summary: "Refresh CI."}}, want: "patch"},
		{name: "feature", previous: "v1.0.0", notes: []releaseNote{{Category: "Added", Summary: "Add APNG."}}, want: "minor"},
		{name: "stable breaking", previous: "v1.0.0", notes: []releaseNote{{Category: "Changed", Breaking: true, Summary: "Change output."}}, want: "major"},
		{name: "pre-one breaking", previous: "v0.8.0", notes: []releaseNote{{Category: "Changed", Breaking: true, Summary: "Change output."}}, want: "minor"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := recommendedVersionBump(test.previous, test.notes); got != test.want {
				t.Fatalf("recommended bump = %q, want %q", got, test.want)
			}
		})
	}
}

func TestGitHubClientReadsPullRequestAndFindsFirstMergedPullRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/owner/project/commits/abcdef012345/pulls":
			_ = json.NewEncoder(response).Encode([]githubPullRequest{{Number: 42}})
		case "/repos/owner/project/pulls/42":
			_ = json.NewEncoder(response).Encode(githubPullRequest{
				Number:  42,
				Title:   "feat: add APNG",
				Body:    "<!-- release-note\ncategory: Added\nbreaking: false\nsummary: Add APNG output.\nnone_reason:\n-->",
				HTMLURL: "https://github.com/owner/project/pull/42",
				User:    githubUser{Login: "new-contributor", Type: "User"},
			})
		case "/search/issues":
			if got, want := request.URL.Query().Get("q"), "repo:owner/project type:pr author:new-contributor is:merged"; got != want {
				t.Fatalf("search query = %q, want %q", got, want)
			}
			if got, want := request.URL.Query().Get("sort"), "created"; got != want {
				t.Fatalf("search sort = %q, want %q", got, want)
			}
			if got, want := request.URL.Query().Get("order"), "asc"; got != want {
				t.Fatalf("search order = %q, want %q", got, want)
			}
			_ = json.NewEncoder(response).Encode(githubPullRequestSearchResult{Items: []githubPullRequest{{Number: 42}}})
		default:
			t.Fatalf("unexpected GitHub API path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	client := githubClient{baseURL: server.URL, client: server.Client()}
	pulls, err := client.pullRequestsForCommit(context.Background(), "owner/project", "abcdef012345")
	if err != nil {
		t.Fatalf("pull requests for commit: %v", err)
	}
	if len(pulls) != 1 || pulls[0].Number != 42 {
		t.Fatalf("pulls = %#v", pulls)
	}
	pull, err := client.pullRequest(context.Background(), "owner/project", 42)
	if err != nil {
		t.Fatalf("pull request: %v", err)
	}
	if !isExternalContributor(pull, "owner") {
		t.Fatalf("pull %#v should be an eligible external contributor", pull)
	}
	first, err := client.firstMergedPullRequest(context.Background(), "owner/project", pull.User.Login)
	if err != nil {
		t.Fatalf("first merged pull request: %v", err)
	}
	if first.Number != pull.Number {
		t.Fatalf("first merged pull = %#v, want #%d", first, pull.Number)
	}
	if _, err := parseReleaseNoteDeclaration(pull.Body); err != nil {
		t.Fatalf("parse pull release note: %v", err)
	}
}

func TestNewContributorExcludesOwnerAndBots(t *testing.T) {
	t.Parallel()

	for _, pull := range []githubPullRequest{
		{User: githubUser{Login: "owner", Type: "User"}},
		{User: githubUser{Login: "dependabot[bot]", Type: "Bot"}},
	} {
		if isExternalContributor(pull, "owner") {
			t.Fatalf("pull %#v must not be listed as a new external contributor", pull)
		}
	}
	if !isExternalContributor(githubPullRequest{User: githubUser{Login: "other", Type: "User"}}, "owner") {
		t.Fatal("an external user should be eligible before historical PR lookup")
	}
}

func TestPrepareRendersBilingualNotesAndIndex(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeReleaseNote(t, root, "README.md", "# Changelog\n\n| Version | Date | Release notes |\n| --- | --- | --- |\n| Unreleased | — | [English](unreleased/en.md) · [简体中文](unreleased/zh-CN.md) |\n| [v1.0.0](https://github.com/FlanChanXwO/pixiv-cli/commits/v1.0.0) | 2026-01-01 | [English](v1.0.0/en.md) · [简体中文](v1.0.0/zh-CN.md) |\n")
	writeReleaseNote(t, root, "README.zh-CN.md", "# 更新日志\n\n| 版本 | 日期 | 发布说明 |\n| --- | --- | --- |\n| 未发布 | — | [English](unreleased/en.md) · [简体中文](unreleased/zh-CN.md) |\n| [v1.0.0](https://github.com/FlanChanXwO/pixiv-cli/commits/v1.0.0) | 2026-01-01 | [English](v1.0.0/en.md) · [简体中文](v1.0.0/zh-CN.md) |\n")
	planPath := filepath.Join(root, "plan.json")
	plan := preparePlan{Entries: []preparedEntry{{
		Category: "Added",
		English:  "Add APNG downloads.",
		Chinese:  "新增 APNG 下载。",
		Sources:  []string{"https://github.com/FlanChanXwO/pixiv-cli/pull/42"},
	}}, NewContributors: []newContributor{{
		Login: "new-contributor", ProfileURL: "https://github.com/new-contributor", PullNumber: 42, PullURL: "https://github.com/FlanChanXwO/pixiv-cli/pull/42",
	}}}
	writeJSONFile(t, planPath, plan)

	if err := prepareRelease(prepareConfig{Version: "1.1.0", Previous: "v1.0.0", Date: "2026-07-30", ChangelogRoot: root, PlanPath: planPath, Apply: true}); err != nil {
		t.Fatalf("prepare release: %v", err)
	}
	if err := validateReleaseDirectory(filepath.Join(root, "v1.1.0"), "1.1.0", "v1.0.0"); err != nil {
		t.Fatalf("validate prepared notes: %v", err)
	}
	english, err := os.ReadFile(filepath.Join(root, "v1.1.0", "en.md"))
	if err != nil {
		t.Fatalf("read English notes: %v", err)
	}
	if !strings.Contains(string(english), "## New Contributors") || !strings.Contains(string(english), "[@new-contributor](https://github.com/new-contributor) made their first contribution in [#42]") {
		t.Fatalf("English notes missing contributor: %s", english)
	}
	index, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if !strings.Contains(string(index), "| [v1.1.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.0...v1.1.0) | 2026-07-30") {
		t.Fatalf("index missing new release row: %s", index)
	}
	chineseIndex, err := os.ReadFile(filepath.Join(root, "README.zh-CN.md"))
	if err != nil {
		t.Fatalf("read Simplified Chinese index: %v", err)
	}
	if !strings.Contains(string(chineseIndex), "| [v1.1.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.0...v1.1.0) | 2026-07-30") {
		t.Fatalf("Simplified Chinese index missing new release row: %s", chineseIndex)
	}
}

func TestPrepareRejectsRepeatedSource(t *testing.T) {
	t.Parallel()

	plan := preparePlan{Entries: []preparedEntry{{
		Category: "Added", English: "Add one.", Chinese: "新增一。", Sources: []string{
			"https://github.com/FlanChanXwO/pixiv-cli/pull/42",
			"https://github.com/FlanChanXwO/pixiv-cli/pull/42",
		},
	}}}
	if err := validatePreparePlan(plan); err == nil || !strings.Contains(err.Error(), "repeats source") {
		t.Fatalf("validate plan error = %v, want repeated source error", err)
	}
}

func TestValidateCoverageRejectsMissingAuditSource(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeReleaseNote(t, root, "en.md", `# v1.2.0 — 2026-07-30

## Added

- Added one. ([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))

**Full Changelog**: [v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)
`)
	writeReleaseNote(t, root, "zh-CN.md", `# v1.2.0 — 2026-07-30

## 新增

- 新增一。([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))

**完整变更**：[v1.1.0...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.2.0)
`)
	report := auditReport{Sources: []auditSource{
		{Kind: "pull_request", URL: "https://github.com/FlanChanXwO/pixiv-cli/pull/42", Note: &releaseNote{Category: "Added", Summary: "Add one."}},
		{Kind: "pull_request", URL: "https://github.com/FlanChanXwO/pixiv-cli/pull/43", Note: &releaseNote{Category: "Fixed", Summary: "Fix two."}},
	}}
	if err := validateSourceCoverage(root, "1.2.0", "v1.1.0", report); err == nil || !strings.Contains(err.Error(), "does not cover") {
		t.Fatalf("source coverage error = %v, want missing source error", err)
	}
}

func TestPreparePlanRequiresAuditedNewContributor(t *testing.T) {
	t.Parallel()

	plan := preparePlan{Entries: []preparedEntry{{
		Category: "Added", English: "Add one.", Chinese: "新增一。", Sources: []string{"https://github.com/FlanChanXwO/pixiv-cli/pull/42"},
	}}}
	report := auditReport{
		Sources:         []auditSource{{Kind: "pull_request", URL: "https://github.com/FlanChanXwO/pixiv-cli/pull/42", Note: &releaseNote{Category: "Added", Summary: "Add one."}}},
		NewContributors: []newContributor{{Login: "new-contributor", ProfileURL: "https://github.com/new-contributor", PullNumber: 42, PullURL: "https://github.com/FlanChanXwO/pixiv-cli/pull/42"}},
	}
	if err := validatePlanCoverage(plan, report); err == nil || !strings.Contains(err.Error(), "new contributor") {
		t.Fatalf("plan coverage error = %v, want missing contributor error", err)
	}
}

func TestPRValidateReadsEventPayload(t *testing.T) {
	t.Parallel()

	eventPath := filepath.Join(t.TempDir(), "event.json")
	writeJSONFile(t, eventPath, map[string]any{"pull_request": map[string]any{"body": "<!-- release-note\ncategory: Documentation\nbreaking: false\nsummary: Clarify the release workflow.\nnone_reason:\n-->"}})
	if err := validatePullRequestEvent(eventPath); err != nil {
		t.Fatalf("validate PR event: %v", err)
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

func TestSyncHistoryCreatesMissingHistoricalReleaseWithoutAssets(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	writeReleaseNote(t, directory, "en.md", "# v0.4.0 — 2026-07-18\n\n## Maintenance\n\n- Historical maintenance. ([`abcdef0`](https://github.com/FlanChanXwO/pixiv-cli/commit/abcdef0123456789))\n\n**Full Changelog**: [v0.3.0...v0.4.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.3.0...v0.4.0)\n")
	writeReleaseNote(t, directory, "zh-CN.md", "# v0.4.0 — 2026-07-18\n\n## 维护\n\n- 历史维护。([`abcdef0`](https://github.com/FlanChanXwO/pixiv-cli/commit/abcdef0123456789))\n\n**完整变更**：[v0.3.0...v0.4.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.3.0...v0.4.0)\n")
	var created githubReleaseWrite
	var createdRelease githubRelease
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/repos/owner/project/releases/tags/v0.4.0" && createdRelease.ID == 0:
			response.WriteHeader(http.StatusNotFound)
			_, _ = response.Write([]byte(`{"message":"Not Found"}`))
		case request.Method == http.MethodPost && request.URL.Path == "/repos/owner/project/releases":
			if err := json.NewDecoder(request.Body).Decode(&created); err != nil {
				t.Fatalf("decode create: %v", err)
			}
			createdRelease = githubRelease{ID: 40, TagName: created.TagName, Name: created.Name, Body: created.Body}
			_ = json.NewEncoder(response).Encode(createdRelease)
		case request.Method == http.MethodGet && request.URL.Path == "/repos/owner/project/releases/tags/v0.4.0":
			_ = json.NewEncoder(response).Encode(createdRelease)
		default:
			t.Fatalf("unexpected %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	err := syncHistoricalRelease(context.Background(), syncHistoryConfig{
		Repository: "owner/project", Version: "0.4.0", Directory: directory, Apply: true,
		Client: githubClient{baseURL: server.URL, client: server.Client()},
	})
	if err != nil {
		t.Fatalf("create historical release: %v", err)
	}
	if created.TagName != "v0.4.0" || created.Name != "v0.4.0" || created.Draft || strings.Contains(created.Body, "assets") {
		t.Fatalf("created release payload = %#v", created)
	}
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestRenderSourceLink(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ source, want string }{
		{"https://github.com/FlanChanXwO/pixiv-cli/pull/42", "[#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42)"},
		{"https://github.com/FlanChanXwO/pixiv-cli/commit/abcdef0123456789", "[`abcdef0`](https://github.com/FlanChanXwO/pixiv-cli/commit/abcdef0123456789)"},
	} {
		if got, err := renderSourceLink(test.source); err != nil || got != test.want {
			t.Fatalf("render source %q = %q, %v; want %q", test.source, got, err, test.want)
		}
	}
	if _, err := renderSourceLink(fmt.Sprintf("https://example.com/%d", 42)); err == nil {
		t.Fatal("unsupported source must fail")
	}
}

func writeReleaseNote(t *testing.T, root, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
