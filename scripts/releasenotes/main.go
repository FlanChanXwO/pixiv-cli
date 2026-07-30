// Command releasenotes audits and validates the versioned changelog contract.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/releasenotesrender"
)

var (
	releaseHeadingPattern  = regexp.MustCompile(`(?m)^# v([0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?)\s+[—-]\s+.+$`)
	sectionPattern         = regexp.MustCompile(`(?m)^##\s+(.+?)\s*$`)
	sourcePattern          = regexp.MustCompile(`https://github\.com/FlanChanXwO/pixiv-cli/(?:pull/[0-9]+|commit/[0-9a-fA-F]{7,64})`)
	linkPattern            = regexp.MustCompile(`\[[^\]]+\]\((https://github\.com/FlanChanXwO/pixiv-cli/(?:compare/[^)\s]+|commits/[^)\s]+))\)`)
	releaseNotePattern     = regexp.MustCompile(`(?s)<!--\s*release-note\s*\n(.*?)-->`)
	semanticVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
	datePattern            = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
)

// releaseNote 是 PR 模板中 release-note 注释的稳定语义模型。它不包含 PR 编号，
// 因为 audit 会将同一声明与 GitHub 返回的 PR 元数据关联。
type releaseNote struct {
	Category   string `json:"category"`
	Breaking   bool   `json:"breaking"`
	Summary    string `json:"summary"`
	NoneReason string `json:"none_reason"`
}

// githubClient 只封装 release-note 流程需要的 GitHub REST 读取。写入历史
// Release 的操作在 sync-history 子命令中单独、显式地处理。
type githubClient struct {
	baseURL string
	token   string
	client  *http.Client
}

type githubUser struct {
	Login   string `json:"login"`
	Type    string `json:"type"`
	HTMLURL string `json:"html_url"`
}

type githubPullRequest struct {
	Number            int        `json:"number"`
	Title             string     `json:"title"`
	Body              string     `json:"body"`
	HTMLURL           string     `json:"html_url"`
	AuthorAssociation string     `json:"author_association"`
	User              githubUser `json:"user"`
}

type auditSource struct {
	Kind       string       `json:"kind"`
	URL        string       `json:"url"`
	PullNumber int          `json:"pull_number,omitempty"`
	Commit     string       `json:"commit,omitempty"`
	Title      string       `json:"title"`
	Author     string       `json:"author"`
	Note       *releaseNote `json:"release_note,omitempty"`
	Issue      string       `json:"issue,omitempty"`
}

type newContributor struct {
	Login      string `json:"login"`
	ProfileURL string `json:"profile_url"`
	PullNumber int    `json:"pull_number"`
	PullURL    string `json:"pull_url"`
}

type auditReport struct {
	Repository             string           `json:"repository"`
	From                   string           `json:"from,omitempty"`
	To                     string           `json:"to"`
	RecommendedVersionBump string           `json:"recommended_version_bump"`
	Sources                []auditSource    `json:"sources"`
	NewContributors        []newContributor `json:"new_contributors"`
}

// preparePlan 是审核后、可纳入 release-prep PR 的编辑输入。工具刻意不从 PR
// 标题自动生成面向用户的文案：维护者需要在此处合并相关变更并提供双语摘要。
type preparePlan struct {
	Entries         []preparedEntry  `json:"entries"`
	NewContributors []newContributor `json:"new_contributors,omitempty"`
}

type preparedEntry struct {
	Category string   `json:"category"`
	Breaking bool     `json:"breaking,omitempty"`
	English  string   `json:"english"`
	Chinese  string   `json:"zh_cn"`
	Sources  []string `json:"sources"`
}

type prepareConfig struct {
	Version       string
	Previous      string
	Date          string
	ChangelogRoot string
	PlanPath      string
	AuditPath     string
	Apply         bool
	Replace       bool
}

type syncHistoryConfig struct {
	Repository string
	Version    string
	Directory  string
	Client     githubClient
	Apply      bool
}

type githubRelease struct {
	ID      int                  `json:"id"`
	TagName string               `json:"tag_name"`
	Name    string               `json:"name"`
	Body    string               `json:"body"`
	Assets  []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name string `json:"name"`
}

type githubReleaseWrite struct {
	TagName string `json:"tag_name,omitempty"`
	Name    string `json:"name,omitempty"`
	Body    string `json:"body"`
	Draft   bool   `json:"draft"`
}

type releaseDocument struct {
	sections []releaseSection
	sources  []string
	compare  string
}

type releaseSection struct {
	name    string
	entries []string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		return errors.New("a subcommand is required: validate, audit, prepare, pr-validate, or sync-history")
	}
	switch arguments[0] {
	case "validate":
		return runValidate(arguments[1:])
	case "audit":
		return runAudit(arguments[1:])
	case "prepare":
		return runPrepare(arguments[1:])
	case "pr-validate":
		return runPullRequestValidate(arguments[1:])
	case "sync-history":
		return runSyncHistory(arguments[1:])
	case "-h", "--help", "help":
		return errors.New("usage: releasenotes validate|audit|prepare|pr-validate|sync-history")
	default:
		return fmt.Errorf("unknown subcommand %q", arguments[0])
	}
}

func runAudit(arguments []string) error {
	flags := flag.NewFlagSet("audit", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	repository := flags.String("repo", "", "GitHub repository in owner/name form")
	from := flags.String("from", "", "exclusive base tag; empty for the initial release")
	to := flags.String("to", "", "inclusive Git ref or tag")
	apiBase := flags.String("api-base", "https://api.github.com", "GitHub REST API base URL")
	tokenEnv := flags.String("token-env", "GH_TOKEN", "environment variable containing an optional GitHub token")
	output := flags.String("output", "", "optional JSON report path")
	requireClassified := flags.Bool("require-classified", false, "fail when a source has no usable release-note declaration")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("audit accepts no positional arguments: %q", flags.Arg(0))
	}
	if !validRepository(*repository) || *to == "" {
		return errors.New("audit requires --repo owner/name and --to")
	}
	report, err := collectAudit(context.Background(), auditConfig{
		repository: *repository,
		from:       *from,
		to:         *to,
		client: githubClient{
			baseURL: *apiBase,
			token:   os.Getenv(*tokenEnv),
			client:  http.DefaultClient,
		},
	})
	if err != nil {
		return err
	}
	if err := writeJSON(*output, report); err != nil {
		return err
	}
	if *requireClassified {
		for _, source := range report.Sources {
			if source.Issue != "" {
				return fmt.Errorf("audit has unresolved source %s: %s", source.URL, source.Issue)
			}
		}
	}
	return nil
}

func runPrepare(arguments []string) error {
	flags := flag.NewFlagSet("prepare", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	version := flags.String("version", "", "semantic version without v")
	previous := flags.String("previous", "", "previous v-prefixed tag; empty for the initial release")
	date := flags.String("date", "", "release date in YYYY-MM-DD form")
	changelogRoot := flags.String("changelog-root", "changelog", "changelog root directory")
	plan := flags.String("plan", "", "reviewed JSON release plan")
	audit := flags.String("audit", "", "optional JSON audit report whose sources must be covered")
	apply := flags.Bool("apply", false, "write the versioned notes and indexes after rendering")
	replace := flags.Bool("replace", false, "replace an existing versioned note pair during an authorized historical backfill")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("prepare accepts no positional arguments: %q", flags.Arg(0))
	}
	if *version == "" || *date == "" || *plan == "" {
		return errors.New("prepare requires --version, --date, and --plan")
	}
	return prepareRelease(prepareConfig{
		Version:       *version,
		Previous:      *previous,
		Date:          *date,
		ChangelogRoot: *changelogRoot,
		PlanPath:      *plan,
		AuditPath:     *audit,
		Apply:         *apply,
		Replace:       *replace,
	})
}

func prepareRelease(config prepareConfig) error {
	if !semanticVersionPattern.MatchString(config.Version) {
		return fmt.Errorf("invalid semantic version %q", config.Version)
	}
	if !datePattern.MatchString(config.Date) {
		return fmt.Errorf("invalid release date %q", config.Date)
	}
	if _, err := time.Parse("2006-01-02", config.Date); err != nil {
		return fmt.Errorf("invalid release date %q: %w", config.Date, err)
	}
	if config.ChangelogRoot == "" || config.PlanPath == "" {
		return errors.New("changelog root and plan path are required")
	}
	if config.Replace && !config.Apply {
		return errors.New("prepare --replace requires --apply")
	}
	plan, err := readPreparePlan(config.PlanPath)
	if err != nil {
		return err
	}
	if err := validatePreparePlan(plan); err != nil {
		return err
	}
	var report *auditReport
	if config.AuditPath != "" {
		parsedReport, err := readAuditReport(config.AuditPath)
		if err != nil {
			return err
		}
		if err := validatePlanCoverage(plan, parsedReport); err != nil {
			return err
		}
		report = &parsedReport
	}
	files, err := renderPreparedRelease(config, plan)
	if err != nil {
		return err
	}
	if !config.Apply {
		for _, path := range sortedFilePaths(files) {
			fmt.Fprintln(os.Stdout, path)
		}
		return nil
	}
	for _, path := range sortedFilePaths(files) {
		if err := writePreparedFile(path, files[path], config.Replace); err != nil {
			return err
		}
	}
	directory := filepath.Join(config.ChangelogRoot, "v"+config.Version)
	if report != nil {
		return validateSourceCoverage(directory, config.Version, config.Previous, *report)
	}
	return validateReleaseDirectory(directory, config.Version, config.Previous)
}

func readPreparePlan(path string) (preparePlan, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return preparePlan{}, fmt.Errorf("read release plan: %w", err)
	}
	var plan preparePlan
	if err := json.Unmarshal(body, &plan); err != nil {
		return preparePlan{}, fmt.Errorf("parse release plan: %w", err)
	}
	return plan, nil
}

func readAuditReport(path string) (auditReport, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return auditReport{}, fmt.Errorf("read audit report: %w", err)
	}
	var report auditReport
	if err := json.Unmarshal(body, &report); err != nil {
		return auditReport{}, fmt.Errorf("parse audit report: %w", err)
	}
	return report, nil
}

func validatePreparePlan(plan preparePlan) error {
	if len(plan.Entries) == 0 {
		return errors.New("release plan has no entries")
	}
	for index, entry := range plan.Entries {
		if _, ok := releaseNoteCategories[entry.Category]; !ok || entry.Category == "None" {
			return fmt.Errorf("entry %d has unsupported category %q", index+1, entry.Category)
		}
		if entry.English == "" || entry.Chinese == "" {
			return fmt.Errorf("entry %d must contain both English and Simplified Chinese text", index+1)
		}
		if len(entry.Sources) == 0 {
			return fmt.Errorf("entry %d has no sources", index+1)
		}
		entrySources := make(map[string]struct{})
		for _, source := range entry.Sources {
			if _, err := renderSourceLink(source); err != nil {
				return fmt.Errorf("entry %d source: %w", index+1, err)
			}
			if _, exists := entrySources[source]; exists {
				return fmt.Errorf("release plan entry %d repeats source %s", index+1, source)
			}
			entrySources[source] = struct{}{}
		}
	}
	contributors := make(map[string]struct{})
	for _, contributor := range plan.NewContributors {
		if contributor.Login == "" || contributor.ProfileURL == "" || contributor.PullNumber <= 0 || contributor.PullURL == "" {
			return errors.New("new contributor record is incomplete")
		}
		if _, err := renderSourceLink(contributor.PullURL); err != nil {
			return fmt.Errorf("new contributor %q: %w", contributor.Login, err)
		}
		if _, exists := contributors[contributor.Login]; exists {
			return fmt.Errorf("release plan repeats new contributor %q", contributor.Login)
		}
		contributors[contributor.Login] = struct{}{}
	}
	return nil
}

func validatePlanCoverage(plan preparePlan, report auditReport) error {
	planned := make(map[string]struct{})
	for _, entry := range plan.Entries {
		for _, source := range entry.Sources {
			planned[source] = struct{}{}
		}
	}
	for _, contributor := range plan.NewContributors {
		planned[contributor.PullURL] = struct{}{}
	}
	for _, source := range report.Sources {
		if source.Kind == "pull_request" && (source.Note == nil || source.Issue != "") {
			return fmt.Errorf("audit source %s is not classified: %s", source.URL, source.Issue)
		}
		if source.Note != nil && source.Note.Category == "None" {
			continue
		}
		if _, ok := planned[source.URL]; !ok {
			return fmt.Errorf("release plan does not cover audited source %s", source.URL)
		}
	}
	plannedContributors := make(map[string]newContributor, len(plan.NewContributors))
	for _, contributor := range plan.NewContributors {
		plannedContributors[contributor.Login] = contributor
	}
	for _, contributor := range report.NewContributors {
		planned, ok := plannedContributors[contributor.Login]
		if !ok {
			return fmt.Errorf("release plan does not include audited new contributor %q", contributor.Login)
		}
		if planned.ProfileURL != contributor.ProfileURL || planned.PullNumber != contributor.PullNumber || planned.PullURL != contributor.PullURL {
			return fmt.Errorf("release plan new contributor %q differs from the audit report", contributor.Login)
		}
		delete(plannedContributors, contributor.Login)
	}
	for login := range plannedContributors {
		return fmt.Errorf("release plan includes new contributor %q absent from the audit report", login)
	}
	return nil
}

func renderPreparedRelease(config prepareConfig, plan preparePlan) (map[string][]byte, error) {
	english, err := renderReleaseDocument(config, plan, false)
	if err != nil {
		return nil, err
	}
	chinese, err := renderReleaseDocument(config, plan, true)
	if err != nil {
		return nil, err
	}
	englishIndex, err := updateChangelogIndex(filepath.Join(config.ChangelogRoot, "README.md"), config.Version, config.Previous, config.Date, config.Replace)
	if err != nil {
		return nil, err
	}
	chineseIndex, err := updateChangelogIndex(filepath.Join(config.ChangelogRoot, "README.zh-CN.md"), config.Version, config.Previous, config.Date, config.Replace)
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(config.ChangelogRoot, "v"+config.Version)
	return map[string][]byte{
		filepath.Join(directory, "en.md"):                      english,
		filepath.Join(directory, "zh-CN.md"):                   chinese,
		filepath.Join(config.ChangelogRoot, "README.md"):       englishIndex,
		filepath.Join(config.ChangelogRoot, "README.zh-CN.md"): chineseIndex,
	}, nil
}

func renderReleaseDocument(config prepareConfig, plan preparePlan, chinese bool) ([]byte, error) {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# v%s — %s\n", config.Version, config.Date)
	sectionEntries := make(map[string][]string)
	for _, entry := range plan.Entries {
		section := entry.Category
		if entry.Breaking {
			section = "Breaking changes"
		}
		if chinese {
			section = chineseSectionName(section)
		}
		text := entry.English
		if chinese {
			text = entry.Chinese
		}
		links := make([]string, 0, len(entry.Sources))
		for _, source := range entry.Sources {
			link, err := renderSourceLink(source)
			if err != nil {
				return nil, err
			}
			links = append(links, link)
		}
		sectionEntries[section] = append(sectionEntries[section], "- "+text+" ("+strings.Join(links, ", ")+")")
	}
	order := englishSectionOrder[:7]
	if chinese {
		order = chineseSectionOrder[:7]
	}
	for _, section := range order {
		entries := sectionEntries[section]
		if len(entries) == 0 {
			continue
		}
		fmt.Fprintf(&builder, "\n## %s\n\n%s\n", section, strings.Join(entries, "\n"))
	}
	if len(plan.NewContributors) > 0 {
		name := "New Contributors"
		if chinese {
			name = "新贡献者"
		}
		fmt.Fprintf(&builder, "\n## %s\n\n", name)
		for _, contributor := range plan.NewContributors {
			link, err := renderSourceLink(contributor.PullURL)
			if err != nil {
				return nil, err
			}
			if chinese {
				fmt.Fprintf(&builder, "- [@%s](%s) 在 %s 中完成首次贡献。\n", contributor.Login, contributor.ProfileURL, link)
			} else {
				fmt.Fprintf(&builder, "- [@%s](%s) made their first contribution in %s.\n", contributor.Login, contributor.ProfileURL, link)
			}
		}
	}
	link := changelogCompareLink(config.Version, config.Previous)
	if chinese {
		fmt.Fprintf(&builder, "\n**完整变更**：[%s](%s)\n", changelogCompareLabel(config.Version, config.Previous), link)
	} else {
		fmt.Fprintf(&builder, "\n**Full Changelog**: [%s](%s)\n", changelogCompareLabel(config.Version, config.Previous), link)
	}
	return []byte(builder.String()), nil
}

func chineseSectionName(english string) string {
	for index, candidate := range englishSectionOrder[:7] {
		if candidate == english {
			return chineseSectionOrder[index]
		}
	}
	return english
}

func renderSourceLink(source string) (string, error) {
	if !sourcePattern.MatchString(source) || sourcePattern.FindString(source) != source {
		return "", fmt.Errorf("unsupported source URL %q", source)
	}
	if pull, ok := strings.CutPrefix(source, "https://github.com/FlanChanXwO/pixiv-cli/pull/"); ok {
		return "[#" + pull + "](" + source + ")", nil
	}
	commit, _ := strings.CutPrefix(source, "https://github.com/FlanChanXwO/pixiv-cli/commit/")
	if len(commit) < 7 {
		return "", fmt.Errorf("commit source URL has a short hash %q", source)
	}
	return "[`" + commit[:7] + "`](" + source + ")", nil
}

func changelogCompareLink(version, previous string) string {
	if previous == "" {
		return "https://github.com/FlanChanXwO/pixiv-cli/commits/v" + version
	}
	return "https://github.com/FlanChanXwO/pixiv-cli/compare/" + previous + "...v" + version
}

func changelogCompareLabel(version, previous string) string {
	if previous == "" {
		return "v" + version + " commits"
	}
	return previous + "...v" + version
}

func updateChangelogIndex(path, version, previous, date string, replace bool) ([]byte, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read changelog index: %w", err)
	}
	content := string(body)
	versionMarker := "v" + version + "]("
	if strings.Contains(content, versionMarker) {
		if replace {
			return body, nil
		}
		return nil, fmt.Errorf("changelog index already contains v%s", version)
	}
	anchor := "| Unreleased | — | [English](unreleased/en.md) · [简体中文](unreleased/zh-CN.md) |\n"
	position := strings.Index(content, anchor)
	if position < 0 {
		return nil, fmt.Errorf("changelog index %s has no Unreleased row", path)
	}
	row := fmt.Sprintf("| [v%s](%s) | %s | [English](v%s/en.md) · [简体中文](v%s/zh-CN.md) |\n", version, changelogCompareLink(version, previous), date, version, version)
	position += len(anchor)
	return []byte(content[:position] + row + content[position:]), nil
}

func sortedFilePaths(files map[string][]byte) []string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func writePreparedFile(path string, body []byte, replace bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		base := filepath.Base(path)
		if base != "README.md" && base != "README.zh-CN.md" && !replace {
			return fmt.Errorf("refusing to replace existing file %s", path)
		}
		return os.WriteFile(path, body, 0o644)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(body); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return nil
}

func runPullRequestValidate(arguments []string) error {
	flags := flag.NewFlagSet("pr-validate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	eventPath := flags.String("event", "", "GitHub pull_request event JSON path")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || *eventPath == "" {
		return errors.New("pr-validate requires --event")
	}
	return validatePullRequestEvent(*eventPath)
}

func validatePullRequestEvent(path string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read pull request event: %w", err)
	}
	var event struct {
		PullRequest struct {
			Body string `json:"body"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("parse pull request event: %w", err)
	}
	_, err = parseReleaseNoteDeclaration(event.PullRequest.Body)
	return err
}

type auditConfig struct {
	repository string
	from       string
	to         string
	client     githubClient
}

func collectAudit(ctx context.Context, config auditConfig) (auditReport, error) {
	commits, err := gitRevisionList(config.from, config.to)
	if err != nil {
		return auditReport{}, err
	}
	owner, _, _ := strings.Cut(config.repository, "/")
	report := auditReport{
		Repository:      config.repository,
		From:            config.from,
		To:              config.to,
		Sources:         make([]auditSource, 0),
		NewContributors: make([]newContributor, 0),
	}
	seenPulls := make(map[int]struct{})
	seenContributors := make(map[string]struct{})
	for _, commit := range commits {
		pulls, err := config.client.pullRequestsForCommit(ctx, config.repository, commit)
		if err != nil {
			return auditReport{}, fmt.Errorf("lookup PRs for commit %s: %w", commit, err)
		}
		if len(pulls) == 0 {
			title, author, detailErr := gitCommitDetail(commit)
			if detailErr != nil {
				return auditReport{}, detailErr
			}
			report.Sources = append(report.Sources, auditSource{
				Kind:   "commit",
				URL:    "https://github.com/" + config.repository + "/commit/" + commit,
				Commit: commit,
				Title:  title,
				Author: author,
				Issue:  "direct commit requires an explicit historical attribution",
			})
			continue
		}
		for _, summary := range pulls {
			if _, exists := seenPulls[summary.Number]; exists {
				continue
			}
			seenPulls[summary.Number] = struct{}{}
			pull, err := config.client.pullRequest(ctx, config.repository, summary.Number)
			if err != nil {
				return auditReport{}, fmt.Errorf("lookup PR #%d: %w", summary.Number, err)
			}
			source := auditSource{
				Kind:       "pull_request",
				URL:        pull.HTMLURL,
				PullNumber: pull.Number,
				Title:      pull.Title,
				Author:     pull.User.Login,
			}
			note, parseErr := parseReleaseNoteDeclaration(pull.Body)
			if parseErr != nil {
				source.Issue = parseErr.Error()
			} else if note.Category == "None" {
				source.Note = &note
			} else {
				source.Note = &note
			}
			report.Sources = append(report.Sources, source)
			if isNewExternalContributor(pull, owner) {
				if _, exists := seenContributors[pull.User.Login]; !exists {
					seenContributors[pull.User.Login] = struct{}{}
					report.NewContributors = append(report.NewContributors, newContributor{
						Login:      pull.User.Login,
						ProfileURL: pull.User.HTMLURL,
						PullNumber: pull.Number,
						PullURL:    pull.HTMLURL,
					})
				}
			}
		}
	}
	notes := make([]releaseNote, 0, len(report.Sources))
	for _, source := range report.Sources {
		if source.Note != nil {
			notes = append(notes, *source.Note)
		}
	}
	report.RecommendedVersionBump = recommendedVersionBump(notes)
	sort.Slice(report.Sources, func(left, right int) bool { return report.Sources[left].URL < report.Sources[right].URL })
	sort.Slice(report.NewContributors, func(left, right int) bool {
		return report.NewContributors[left].Login < report.NewContributors[right].Login
	})
	return report, nil
}

func (client githubClient) pullRequestsForCommit(ctx context.Context, repository, commit string) ([]githubPullRequest, error) {
	var pulls []githubPullRequest
	if err := client.getJSON(ctx, "/repos/"+repository+"/commits/"+url.PathEscape(commit)+"/pulls", &pulls); err != nil {
		return nil, err
	}
	return pulls, nil
}

func (client githubClient) pullRequest(ctx context.Context, repository string, number int) (githubPullRequest, error) {
	var pull githubPullRequest
	if err := client.getJSON(ctx, fmt.Sprintf("/repos/%s/pulls/%d", repository, number), &pull); err != nil {
		return githubPullRequest{}, err
	}
	return pull, nil
}

func (client githubClient) getJSON(ctx context.Context, path string, destination any) error {
	return client.requestJSON(ctx, http.MethodGet, path, nil, destination)
}

type githubHTTPError struct {
	Path       string
	StatusCode int
	Body       string
}

func (err *githubHTTPError) Error() string {
	if err.Body == "" {
		return fmt.Sprintf("GitHub API %s: HTTP %d", err.Path, err.StatusCode)
	}
	return fmt.Sprintf("GitHub API %s: HTTP %d: %s", err.Path, err.StatusCode, err.Body)
}

func (client githubClient) requestJSON(ctx context.Context, method, path string, input, destination any) error {
	baseURL := strings.TrimRight(client.baseURL, "/")
	if baseURL == "" {
		return errors.New("GitHub API base URL is required")
	}
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode GitHub API request: %w", err)
		}
		body = strings.NewReader(string(encoded))
	}
	request, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if client.token != "" {
		request.Header.Set("Authorization", "Bearer "+client.token)
	}
	httpClient := client.client
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		responseBody, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			return &githubHTTPError{Path: path, StatusCode: response.StatusCode}
		}
		return &githubHTTPError{Path: path, StatusCode: response.StatusCode, Body: strings.TrimSpace(string(responseBody))}
	}
	if destination == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(destination)
}

func runSyncHistory(arguments []string) error {
	flags := flag.NewFlagSet("sync-history", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	repository := flags.String("repo", "", "GitHub repository in owner/name form")
	version := flags.String("version", "", "semantic version without v")
	directory := flags.String("dir", "", "version directory containing en.md and zh-CN.md")
	apiBase := flags.String("api-base", "https://api.github.com", "GitHub REST API base URL")
	tokenEnv := flags.String("token-env", "GH_TOKEN", "environment variable containing a GitHub token")
	apply := flags.Bool("apply", false, "create or update the historical GitHub Release body")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || !validRepository(*repository) || !semanticVersionPattern.MatchString(*version) || *directory == "" {
		return errors.New("sync-history requires --repo, --version, and --dir")
	}
	return syncHistoricalRelease(context.Background(), syncHistoryConfig{
		Repository: *repository,
		Version:    *version,
		Directory:  *directory,
		Apply:      *apply,
		Client: githubClient{
			baseURL: *apiBase,
			token:   os.Getenv(*tokenEnv),
			client:  http.DefaultClient,
		},
	})
}

// syncHistoricalRelease 只读取或更新 Release 正文。它不接触 tag、draft 状态和
// assets，因此补写历史文本不会补造或替换任何已发布的二进制资产。
func syncHistoricalRelease(ctx context.Context, config syncHistoryConfig) error {
	body, err := renderGitHubReleaseBody(config.Directory, config.Version)
	if err != nil {
		return err
	}
	path := "/repos/" + config.Repository + "/releases/tags/v" + config.Version
	var release githubRelease
	err = config.Client.getJSON(ctx, path, &release)
	if err != nil {
		var apiErr *githubHTTPError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
			return err
		}
		if !config.Apply {
			fmt.Fprintf(os.Stdout, "would create historical Release v%s without assets\n", config.Version)
			return nil
		}
		created := githubRelease{}
		if err := config.Client.requestJSON(ctx, http.MethodPost, "/repos/"+config.Repository+"/releases", githubReleaseWrite{
			TagName: "v" + config.Version,
			Name:    "v" + config.Version,
			Body:    body,
			Draft:   false,
		}, &created); err != nil {
			return fmt.Errorf("create historical Release v%s: %w", config.Version, err)
		}
		release = created
	} else {
		if !config.Apply {
			fmt.Fprintf(os.Stdout, "would update historical Release v%s body; assets remain unchanged\n", config.Version)
			return nil
		}
		if err := config.Client.requestJSON(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/releases/%d", config.Repository, release.ID), githubReleaseWrite{Body: body}, &release); err != nil {
			return fmt.Errorf("update historical Release v%s: %w", config.Version, err)
		}
	}
	var verified githubRelease
	if err := config.Client.getJSON(ctx, path, &verified); err != nil {
		return fmt.Errorf("verify historical Release v%s: %w", config.Version, err)
	}
	if verified.TagName != "v"+config.Version {
		return fmt.Errorf("verified historical Release tag %q, want v%s", verified.TagName, config.Version)
	}
	if verified.Body != body {
		return fmt.Errorf("verified historical Release v%s body differs from local rendering", config.Version)
	}
	return nil
}

func renderGitHubReleaseBody(directory, version string) (string, error) {
	english, err := releasenotesrender.NotesFromChangelog(filepath.Join(directory, "en.md"), version)
	if err != nil {
		return "", fmt.Errorf("English changelog: %w", err)
	}
	chinese, err := releasenotesrender.NotesFromChangelog(filepath.Join(directory, "zh-CN.md"), version)
	if err != nil {
		return "", fmt.Errorf("Simplified Chinese changelog: %w", err)
	}
	return string(releasenotesrender.BilingualBody(english, chinese)), nil
}

func isNewExternalContributor(pull githubPullRequest, owner string) bool {
	if pull.AuthorAssociation != "FIRST_TIME_CONTRIBUTOR" || pull.User.Login == "" || pull.User.Login == owner {
		return false
	}
	return pull.User.Type != "Bot" && !strings.HasSuffix(strings.ToLower(pull.User.Login), "[bot]")
}

func gitRevisionList(from, to string) ([]string, error) {
	if to == "" {
		return nil, errors.New("git revision target is required")
	}
	revision := to
	if from != "" {
		revision = from + ".." + to
	}
	output, err := exec.Command("git", "rev-list", "--reverse", revision).Output()
	if err != nil {
		return nil, fmt.Errorf("list commits for %s: %w", revision, err)
	}
	lines := strings.Fields(string(output))
	if len(lines) == 0 {
		return nil, fmt.Errorf("no commits found for %s", revision)
	}
	return lines, nil
}

func gitCommitDetail(commit string) (title, author string, err error) {
	output, err := exec.Command("git", "show", "-s", "--format=%s%x00%an", commit).Output()
	if err != nil {
		return "", "", fmt.Errorf("read direct commit %s: %w", commit, err)
	}
	title, author, ok := strings.Cut(strings.TrimSuffix(string(output), "\n"), "\x00")
	if !ok || title == "" || author == "" {
		return "", "", fmt.Errorf("read direct commit %s: malformed git metadata", commit)
	}
	return title, author, nil
}

func validRepository(repository string) bool {
	owner, name, ok := strings.Cut(repository, "/")
	return ok && owner != "" && name != "" && !strings.Contains(name, "/")
}

func writeJSON(path string, value any) error {
	var destination io.Writer = os.Stdout
	var file *os.File
	if path != "" {
		opened, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		file = opened
		destination = file
		defer file.Close()
	}
	encoder := json.NewEncoder(destination)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func runValidate(arguments []string) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	version := flags.String("version", "", "semantic version without v")
	directory := flags.String("dir", "", "directory containing en.md and zh-CN.md")
	previous := flags.String("previous", "", "previous v-prefixed tag; empty for the initial release")
	auditPath := flags.String("audit", "", "optional audit JSON report whose source set must match")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("validate accepts no positional arguments: %q", flags.Arg(0))
	}
	if *version == "" || *directory == "" {
		return errors.New("validate requires --version and --dir")
	}
	if *auditPath == "" {
		return validateReleaseDirectory(*directory, *version, *previous)
	}
	report, err := readAuditReport(*auditPath)
	if err != nil {
		return err
	}
	return validateSourceCoverage(*directory, *version, *previous, report)
}

// validateReleaseDirectory 检查可发布的双语版本文件。它不依赖网络，因此既可用于
// 文档 CI，也可用于不可变 tag 的发布前门禁。
func validateReleaseDirectory(directory, version, previous string) error {
	english, err := readReleaseDocument(filepath.Join(directory, "en.md"), version, englishSectionOrder)
	if err != nil {
		return fmt.Errorf("English changelog: %w", err)
	}
	chinese, err := readReleaseDocument(filepath.Join(directory, "zh-CN.md"), version, chineseSectionOrder)
	if err != nil {
		return fmt.Errorf("Simplified Chinese changelog: %w", err)
	}
	if !sameStrings(english.sources, chinese.sources) {
		return fmt.Errorf("bilingual source sets differ: English=%v SimplifiedChinese=%v", english.sources, chinese.sources)
	}
	if err := validateCompareLink(english.compare, version, previous); err != nil {
		return fmt.Errorf("English Full Changelog: %w", err)
	}
	if err := validateCompareLink(chinese.compare, version, previous); err != nil {
		return fmt.Errorf("Simplified Chinese Full Changelog: %w", err)
	}
	if english.compare != chinese.compare {
		return fmt.Errorf("bilingual compare links differ: English=%q SimplifiedChinese=%q", english.compare, chinese.compare)
	}
	return nil
}

// validateSourceCoverage 将已经渲染的说明与审计报告逐项对照。它既能发现漏记的
// PR/历史 direct commit，也拒绝无法由该版本审计报告解释的额外来源链接。
func validateSourceCoverage(directory, version, previous string, report auditReport) error {
	if err := validateReleaseDirectory(directory, version, previous); err != nil {
		return err
	}
	english, err := readReleaseDocument(filepath.Join(directory, "en.md"), version, englishSectionOrder)
	if err != nil {
		return err
	}
	expected := make(map[string]struct{})
	for _, source := range report.Sources {
		if source.Kind == "pull_request" {
			if source.Note == nil || source.Issue != "" {
				return fmt.Errorf("audit source %s is not classified: %s", source.URL, source.Issue)
			}
			if source.Note.Category == "None" {
				continue
			}
		}
		expected[source.URL] = struct{}{}
	}
	for _, contributor := range report.NewContributors {
		expected[contributor.PullURL] = struct{}{}
	}
	actual := make(map[string]struct{}, len(english.sources))
	for _, source := range english.sources {
		actual[source] = struct{}{}
	}
	for source := range expected {
		if _, ok := actual[source]; !ok {
			return fmt.Errorf("release notes does not cover audited source %s", source)
		}
	}
	for source := range actual {
		if _, ok := expected[source]; !ok {
			return fmt.Errorf("release notes source %s is not present in the audit report", source)
		}
	}
	return nil
}

var englishSectionOrder = []string{
	"Breaking changes",
	"Added",
	"Changed",
	"Fixed",
	"Security",
	"Documentation",
	"Maintenance",
	"New Contributors",
}

var chineseSectionOrder = []string{
	"破坏性变更",
	"新增",
	"变更",
	"修复",
	"安全",
	"文档",
	"维护",
	"新贡献者",
}

func readReleaseDocument(path, version string, sectionOrder []string) (releaseDocument, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return releaseDocument{}, err
	}
	content := strings.TrimSpace(string(body))
	heading := releaseHeadingPattern.FindStringSubmatch(content)
	if len(heading) != 2 || heading[1] != version {
		return releaseDocument{}, fmt.Errorf("must start with heading for v%s", version)
	}

	matches := sectionPattern.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return releaseDocument{}, errors.New("has no release-note sections")
	}
	allowed := make(map[string]int, len(sectionOrder))
	for index, name := range sectionOrder {
		allowed[name] = index
	}
	document := releaseDocument{}
	previousIndex := -1
	for index, match := range matches {
		name := strings.TrimSpace(content[match[2]:match[3]])
		order, ok := allowed[name]
		if !ok {
			return releaseDocument{}, fmt.Errorf("contains unsupported section %q", name)
		}
		if order <= previousIndex {
			return releaseDocument{}, fmt.Errorf("section %q is out of order", name)
		}
		previousIndex = order
		end := len(content)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}
		entries := bulletEntries(content[match[1]:end])
		if len(entries) == 0 {
			return releaseDocument{}, fmt.Errorf("section %q has no entries", name)
		}
		for _, entry := range entries {
			sources := sourcePattern.FindAllString(entry, -1)
			if len(sources) == 0 {
				return releaseDocument{}, fmt.Errorf("entry in section %q has no source link", name)
			}
			document.sources = append(document.sources, sources...)
		}
		document.sections = append(document.sections, releaseSection{name: name, entries: entries})
	}

	links := linkPattern.FindAllStringSubmatch(content, -1)
	if len(links) != 1 {
		return releaseDocument{}, errors.New("must contain exactly one Full Changelog link")
	}
	document.compare = links[0][1]
	document.sources = sortedUnique(document.sources)
	return document, nil
}

func bulletEntries(section string) []string {
	lines := strings.Split(section, "\n")
	entries := make([]string, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			entries = append(entries, strings.TrimSpace(strings.TrimPrefix(line, "- ")))
		}
	}
	return entries
}

func validateCompareLink(link, version, previous string) error {
	if previous == "" {
		expected := "https://github.com/FlanChanXwO/pixiv-cli/commits/v" + version
		if link != expected {
			return fmt.Errorf("got %q, want %q", link, expected)
		}
		return nil
	}
	expected := "https://github.com/FlanChanXwO/pixiv-cli/compare/" + previous + "...v" + version
	if link != expected {
		return fmt.Errorf("got %q, want %q", link, expected)
	}
	return nil
}

func sortedUnique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// parseReleaseNoteDeclaration 读取 PR 正文中的机器可读注释。注释不直接显示在
// GitHub 页面上，既避免重复面向读者的描述，也能让 CI 在离线事件载荷中稳定校验。
func parseReleaseNoteDeclaration(body string) (releaseNote, error) {
	match := releaseNotePattern.FindStringSubmatch(body)
	if len(match) != 2 {
		return releaseNote{}, errors.New("missing release-note declaration")
	}
	values := make(map[string]string)
	for _, rawLine := range strings.Split(match[1], "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return releaseNote{}, fmt.Errorf("invalid release-note line %q", line)
		}
		key = strings.TrimSpace(key)
		if _, exists := values[key]; exists {
			return releaseNote{}, fmt.Errorf("release-note repeats %q", key)
		}
		values[key] = strings.TrimSpace(value)
	}
	for _, key := range []string{"category", "breaking", "summary", "none_reason"} {
		if _, ok := values[key]; !ok {
			return releaseNote{}, fmt.Errorf("release-note is missing %s", key)
		}
	}
	for key := range values {
		switch key {
		case "category", "breaking", "summary", "none_reason":
		default:
			return releaseNote{}, fmt.Errorf("release-note contains unsupported key %q", key)
		}
	}
	if _, ok := releaseNoteCategories[values["category"]]; !ok {
		return releaseNote{}, fmt.Errorf("release-note has unsupported category %q", values["category"])
	}
	breaking, err := strconv.ParseBool(values["breaking"])
	if err != nil {
		return releaseNote{}, fmt.Errorf("release-note breaking: %w", err)
	}
	note := releaseNote{
		Category:   values["category"],
		Breaking:   breaking,
		Summary:    values["summary"],
		NoneReason: values["none_reason"],
	}
	if note.Summary == "" {
		return releaseNote{}, errors.New("release-note summary is required")
	}
	if note.Category == "None" {
		if note.Breaking {
			return releaseNote{}, errors.New("release-note None category cannot be breaking")
		}
		if note.NoneReason == "" {
			return releaseNote{}, errors.New("release-note none_reason is required for category None")
		}
	} else if note.NoneReason != "" {
		return releaseNote{}, errors.New("release-note none_reason is only valid for category None")
	}
	return note, nil
}

var releaseNoteCategories = map[string]struct{}{
	"Added":         {},
	"Changed":       {},
	"Fixed":         {},
	"Security":      {},
	"Documentation": {},
	"Maintenance":   {},
	"None":          {},
}

func recommendedVersionBump(notes []releaseNote) string {
	bump := "none"
	for _, note := range notes {
		if note.Category == "None" {
			continue
		}
		if note.Breaking {
			return "major"
		}
		if note.Category == "Added" {
			bump = "minor"
			continue
		}
		if bump == "none" {
			bump = "patch"
		}
	}
	return bump
}
