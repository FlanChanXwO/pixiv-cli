// Package releasenotes audits and validates the versioned changelog contract.
package releasenotes

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
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/releasenotesrender"
)

var (
	releaseHeadingPattern  = regexp.MustCompile(`(?m)^# v([0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?)\s+[—-]\s+.+$`)
	sectionPattern         = regexp.MustCompile(`(?m)^##\s+(.+?)\s*$`)
	sourcePattern          = regexp.MustCompile(`https://github\.com/FlanChanXwO/pixiv-cli/(?:pull/[0-9]+|commit/[0-9a-fA-F]{7,64})`)
	linkPattern            = regexp.MustCompile(`\[[^\]]+\]\((https://github\.com/FlanChanXwO/pixiv-cli/(?:compare/[^)\s]+|commits/[^)\s]+))\)`)
	semanticVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
)

// githubClient 只封装发布流程需要的 GitHub REST 读取。写入历史
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
	Number  int        `json:"number"`
	Title   string     `json:"title"`
	HTMLURL string     `json:"html_url"`
	User    githubUser `json:"user"`
}

type githubPullRequestSearchResult struct {
	Items []githubPullRequest `json:"items"`
}

type auditSource struct {
	Kind       string `json:"kind"`
	URL        string `json:"url"`
	PullNumber int    `json:"pull_number,omitempty"`
	Commit     string `json:"commit,omitempty"`
	Title      string `json:"title"`
	Author     string `json:"author"`
}

type newContributor struct {
	Login      string `json:"login"`
	ProfileURL string `json:"profile_url"`
	PullNumber int    `json:"pull_number"`
	PullURL    string `json:"pull_url"`
}

type auditReport struct {
	Repository      string           `json:"repository"`
	From            string           `json:"from,omitempty"`
	To              string           `json:"to"`
	Sources         []auditSource    `json:"sources"`
	NewContributors []newContributor `json:"new_contributors"`
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

// Run 是 scripts/cmd/releasenotes 的入口 owner：解析参数并委托给 changelog 校验逻辑。
func Run(args []string) error {
	return run(args)
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		return errors.New("a subcommand is required: validate, audit, or sync-history")
	}
	switch arguments[0] {
	case "validate":
		return runValidate(arguments[1:])
	case "audit":
		return runAudit(arguments[1:])
	case "sync-history":
		return runSyncHistory(arguments[1:])
	case "-h", "--help", "help":
		return errors.New("usage: releasenotes validate|audit|sync-history")
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
	return nil
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
	firstMergedPulls := make(map[string]int)
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
			report.Sources = append(report.Sources, auditSource{
				Kind:       "pull_request",
				URL:        pull.HTMLURL,
				PullNumber: pull.Number,
				Title:      pull.Title,
				Author:     pull.User.Login,
			})
			if isExternalContributor(pull, owner) {
				firstMergedPull, known := firstMergedPulls[pull.User.Login]
				if !known {
					// author_association 会随仓库历史变化，故以仓库中该作者
					// 最早合并的 PR 为准，并缓存结果避免同一作者重复查询。
					first, firstErr := config.client.firstMergedPullRequest(ctx, config.repository, pull.User.Login)
					if firstErr != nil {
						return auditReport{}, fmt.Errorf("lookup first merged PR for %q: %w", pull.User.Login, firstErr)
					}
					firstMergedPull = first.Number
					firstMergedPulls[pull.User.Login] = firstMergedPull
				}
				if firstMergedPull != pull.Number {
					continue
				}
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

// firstMergedPullRequest 使用仓库的完整 PR 历史，而不是当前 author_association，
// 判定某个作者最早合并的 PR。该查询只取排序后的首项，不依赖任意分页上限。
func (client githubClient) firstMergedPullRequest(ctx context.Context, repository, login string) (githubPullRequest, error) {
	query := url.Values{}
	query.Set("q", "repo:"+repository+" type:pr author:"+login+" is:merged")
	query.Set("sort", "created")
	query.Set("order", "asc")
	query.Set("per_page", "1")
	var result githubPullRequestSearchResult
	if err := client.getJSON(ctx, "/search/issues?"+query.Encode(), &result); err != nil {
		return githubPullRequest{}, err
	}
	if len(result.Items) == 0 {
		return githubPullRequest{}, fmt.Errorf("no merged pull request found")
	}
	return result.Items[0], nil
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

func isExternalContributor(pull githubPullRequest, owner string) bool {
	if pull.User.Login == "" || pull.User.Login == owner {
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
		expected[source.URL] = struct{}{}
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
