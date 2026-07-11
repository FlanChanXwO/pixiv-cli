package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
)

const (
	defaultGitHubAPIBaseURL = "https://api.github.com"
	defaultGitHubRepository = "FlanChanXwO/pixiv-cli"
	releaseCacheFilename    = "github-releases.json"
	// releaseCacheSchemaVersion 2 起缓存页保留 Release assets；旧 schema 不能接受 304，
	// 否则显式 self-update 会永久看到缺少资产，直到用户手动删除缓存。
	releaseCacheSchemaVersion = 2
	automaticCheckTimeout     = 3 * time.Second
)

// ReleaseClientOptions 配置 GitHub Releases 客户端。
// APIBaseURL、HTTPClient、CacheDir 和 Now 允许调用方为受控环境注入依赖。
type ReleaseClientOptions struct {
	APIBaseURL string
	Repository string
	HTTPClient *http.Client
	CacheDir   string
	Now        func() time.Time
}

// ReleaseCheckOptions 表示一次 Releases 查询的渠道与触发方式。
type ReleaseCheckOptions struct {
	IncludePrerelease bool
	Automatic         bool
}

// Release 是可用于更新的 GitHub Release 的最小公开描述。
type Release struct {
	TagName    string
	Version    string
	Prerelease bool
	Assets     []ReleaseAsset
}

// ReleaseAsset 是 GitHub Release 附带的一个可下载资产。
// 仅使用 Releases API 返回的 browser_download_url，避免根据 tag 猜测下载地址。
type ReleaseAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

// ReleaseCheckResult 是一次查询得到的候选 Release。
type ReleaseCheckResult struct {
	Release   *Release
	Throttled bool
}

// GitHubReleaseClient 通过 GitHub Releases API 查询候选版本。
type GitHubReleaseClient struct {
	endpoint   *url.URL
	httpClient *http.Client
	cacheDir   string
	cachePath  string
	now        func() time.Time
}

// NewGitHubReleaseClient 建立唯一使用 GitHub Releases API 的更新查询客户端。
func NewGitHubReleaseClient(options ReleaseClientOptions) (*GitHubReleaseClient, error) {
	apiBaseURL := options.APIBaseURL
	if apiBaseURL == "" {
		apiBaseURL = defaultGitHubAPIBaseURL
	}
	baseURL, err := url.Parse(apiBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub API base URL %q: %w", apiBaseURL, err)
	}
	if baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("GitHub API base URL %q must be absolute", apiBaseURL)
	}

	repository := options.Repository
	if repository == "" {
		repository = defaultGitHubRepository
	}
	owner, name, ok := strings.Cut(repository, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return nil, fmt.Errorf("GitHub repository %q must have owner/name form", repository)
	}
	endpoint := baseURL.JoinPath("repos", owner, name, "releases")

	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	cacheDir := options.CacheDir
	if cacheDir == "" {
		userCacheDir, err := os.UserCacheDir()
		if err != nil {
			return nil, fmt.Errorf("determine user cache directory: %w", err)
		}
		cacheDir = filepath.Join(userCacheDir, "pixiv-cli")
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &GitHubReleaseClient{
		endpoint:   endpoint,
		httpClient: httpClient,
		cacheDir:   cacheDir,
		cachePath:  filepath.Join(cacheDir, releaseCacheFilename),
		now:        now,
	}, nil
}

// Check 从 GitHub Releases API 选择符合渠道的最高 SemVer Release。
func (c *GitHubReleaseClient) Check(ctx context.Context, options ReleaseCheckOptions) (ReleaseCheckResult, error) {
	if c == nil {
		return ReleaseCheckResult{}, fmt.Errorf("GitHub release client is nil")
	}
	if options.Automatic {
		// 产品要求普通命令后的自动更新检查最多占用三秒；显式 update 不使用此短时限。
		automaticContext, cancel := context.WithTimeout(ctx, automaticCheckTimeout)
		defer cancel()
		ctx = automaticContext
	}
	cache, cacheExists, err := c.readCache(ctx)
	if err != nil {
		return ReleaseCheckResult{}, err
	}
	if err := checkContext(ctx, "read GitHub Releases cache"); err != nil {
		return ReleaseCheckResult{}, err
	}
	now := c.now()
	if options.Automatic && cacheExists && cache.SchemaVersion == releaseCacheSchemaVersion && now.Before(cache.CheckedAt.Add(24*time.Hour)) {
		selected, err := selectRelease(cache.Releases, options.IncludePrerelease)
		if err != nil {
			return ReleaseCheckResult{}, err
		}
		return ReleaseCheckResult{Release: selected, Throttled: true}, nil
	}
	refreshedCache, err := c.fetchReleaseCache(ctx, cache)
	if err != nil {
		return ReleaseCheckResult{}, err
	}
	if err := checkContext(ctx, "fetch GitHub Releases pages"); err != nil {
		return ReleaseCheckResult{}, err
	}
	selected, err := selectRelease(refreshedCache.Releases, options.IncludePrerelease)
	if err != nil {
		return ReleaseCheckResult{}, err
	}
	refreshedCache.CheckedAt = now
	if err := c.writeCache(refreshedCache); err != nil {
		return ReleaseCheckResult{}, err
	}
	return ReleaseCheckResult{Release: selected}, nil
}

type githubRelease struct {
	TagName    string         `json:"tag_name"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Assets     []ReleaseAsset `json:"assets"`
}

type releaseCache struct {
	SchemaVersion int                `json:"schema_version"`
	CheckedAt     time.Time          `json:"checked_at"`
	Releases      []githubRelease    `json:"releases"`
	Pages         []releaseCachePage `json:"pages,omitempty"`
}

type releaseCachePage struct {
	URL      string          `json:"url"`
	ETag     string          `json:"etag,omitempty"`
	NextURL  string          `json:"next_url,omitempty"`
	Releases []githubRelease `json:"releases"`
}

func (c *GitHubReleaseClient) readCache(ctx context.Context) (releaseCache, bool, error) {
	if err := checkContext(ctx, "read GitHub Releases cache"); err != nil {
		return releaseCache{}, false, err
	}
	cacheBytes, err := os.ReadFile(c.cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return releaseCache{}, false, nil
		}
		return releaseCache{}, false, fmt.Errorf("read GitHub Releases cache %q: %w", c.cachePath, err)
	}
	var cache releaseCache
	if err := json.Unmarshal(cacheBytes, &cache); err != nil {
		return releaseCache{}, false, fmt.Errorf("decode GitHub Releases cache %q: %w", c.cachePath, err)
	}
	if cache.CheckedAt.IsZero() {
		return releaseCache{}, false, fmt.Errorf("decode GitHub Releases cache %q: missing checked_at", c.cachePath)
	}
	if err := checkContext(ctx, "read GitHub Releases cache"); err != nil {
		return releaseCache{}, false, err
	}
	return cache, true, nil
}

func checkContext(ctx context.Context, action string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	return nil
}

func (c *GitHubReleaseClient) fetchReleaseCache(ctx context.Context, previous releaseCache) (releaseCache, error) {
	cachedPages := make(map[string]releaseCachePage, len(previous.Pages))
	if previous.SchemaVersion == releaseCacheSchemaVersion {
		for _, page := range previous.Pages {
			pageURL, err := c.parseReleasePageURL(page.URL)
			if err != nil {
				return releaseCache{}, fmt.Errorf("validate cached GitHub Releases page %q: %w", page.URL, err)
			}
			pageID := canonicalReleasePageURL(pageURL)
			if _, duplicate := cachedPages[pageID]; duplicate {
				return releaseCache{}, fmt.Errorf("cached GitHub Releases page %q appears more than once", page.URL)
			}
			cachedPages[pageID] = page
		}
	}

	current := *c.endpoint
	visited := make(map[string]struct{})
	refreshed := releaseCache{SchemaVersion: releaseCacheSchemaVersion}
	for {
		if err := checkContext(ctx, "fetch GitHub Releases page"); err != nil {
			return releaseCache{}, err
		}
		pageID := canonicalReleasePageURL(&current)
		if _, seen := visited[pageID]; seen {
			return releaseCache{}, fmt.Errorf("GitHub Releases pagination loop at %q", pageID)
		}
		visited[pageID] = struct{}{}

		cachedPage, hasCachedPage := cachedPages[pageID]
		page, nextPage, err := c.fetchReleasePage(ctx, &current, cachedPage, hasCachedPage)
		if err != nil {
			return releaseCache{}, err
		}
		refreshed.Pages = append(refreshed.Pages, page)
		refreshed.Releases = append(refreshed.Releases, page.Releases...)
		if nextPage == nil {
			return refreshed, nil
		}
		current = *nextPage
	}
}

func (c *GitHubReleaseClient) fetchReleasePage(ctx context.Context, pageURL *url.URL, cachedPage releaseCachePage, hasCachedPage bool) (releaseCachePage, *url.URL, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL.String(), nil)
	if err != nil {
		return releaseCachePage{}, nil, fmt.Errorf("create GitHub Releases request: %w", err)
	}
	if hasCachedPage && cachedPage.ETag != "" {
		request.Header.Set("If-None-Match", cachedPage.ETag)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return releaseCachePage{}, nil, fmt.Errorf("request GitHub Releases page %q: %w", pageURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified {
		if !hasCachedPage {
			return releaseCachePage{}, nil, fmt.Errorf("GitHub Releases page %q returned HTTP 304 without a cached response", pageURL)
		}
		nextPage, err := c.cachedNextReleasePage(cachedPage.NextURL)
		if err != nil {
			return releaseCachePage{}, nil, fmt.Errorf("read cached next GitHub Releases page after %q: %w", pageURL, err)
		}
		return cachedPage, nextPage, nil
	}
	if response.StatusCode != http.StatusOK {
		return releaseCachePage{}, nil, fmt.Errorf("GitHub Releases page %q returned HTTP %s", pageURL, response.Status)
	}

	var releases []githubRelease
	if err := json.NewDecoder(response.Body).Decode(&releases); err != nil {
		return releaseCachePage{}, nil, fmt.Errorf("decode GitHub Releases page %q: %w", pageURL, err)
	}
	if err := checkContext(ctx, "decode GitHub Releases page"); err != nil {
		return releaseCachePage{}, nil, err
	}
	nextPage, err := c.nextReleasePage(response.Header.Values("Link"), pageURL)
	if err != nil {
		return releaseCachePage{}, nil, fmt.Errorf("parse next GitHub Releases page after %q: %w", pageURL, err)
	}
	nextURL := ""
	if nextPage != nil {
		nextURL = nextPage.String()
	}
	return releaseCachePage{
		URL:      pageURL.String(),
		ETag:     response.Header.Get("ETag"),
		NextURL:  nextURL,
		Releases: publishedReleases(releases),
	}, nextPage, nil
}

func (c *GitHubReleaseClient) cachedNextReleasePage(nextURL string) (*url.URL, error) {
	if nextURL == "" {
		return nil, nil
	}
	return c.parseReleasePageURL(nextURL)
}

func (c *GitHubReleaseClient) nextReleasePage(linkHeaders []string, current *url.URL) (*url.URL, error) {
	var nextURL *url.URL
	for _, linkHeader := range linkHeaders {
		for _, link := range strings.Split(linkHeader, ",") {
			link = strings.TrimSpace(link)
			if link == "" {
				return nil, fmt.Errorf("empty Link value")
			}
			parts := strings.Split(link, ";")
			if len(parts) == 0 || len(parts[0]) < 3 || parts[0][0] != '<' || parts[0][len(parts[0])-1] != '>' {
				return nil, fmt.Errorf("invalid Link value %q", link)
			}
			isNext := false
			for _, parameter := range parts[1:] {
				name, value, hasValue := strings.Cut(strings.TrimSpace(parameter), "=")
				if !hasValue {
					return nil, fmt.Errorf("invalid Link parameter %q", parameter)
				}
				if strings.EqualFold(strings.TrimSpace(name), "rel") {
					value = strings.Trim(strings.TrimSpace(value), "\"")
					for _, relation := range strings.Fields(value) {
						if relation == "next" {
							isNext = true
						}
					}
				}
			}
			if !isNext {
				continue
			}
			if nextURL != nil {
				return nil, fmt.Errorf("more than one next Link")
			}
			reference, err := url.Parse(parts[0][1 : len(parts[0])-1])
			if err != nil {
				return nil, fmt.Errorf("parse Link URL: %w", err)
			}
			nextURL, err = c.validateReleasePageURL(current.ResolveReference(reference))
			if err != nil {
				return nil, err
			}
		}
	}
	return nextURL, nil
}

func (c *GitHubReleaseClient) parseReleasePageURL(value string) (*url.URL, error) {
	pageURL, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	return c.validateReleasePageURL(pageURL)
}

func (c *GitHubReleaseClient) validateReleasePageURL(pageURL *url.URL) (*url.URL, error) {
	if !pageURL.IsAbs() {
		return nil, fmt.Errorf("URL %q is not absolute", pageURL)
	}
	if pageURL.Fragment != "" {
		return nil, fmt.Errorf("URL %q has a fragment", pageURL)
	}
	if pageURL.Scheme != c.endpoint.Scheme || pageURL.Host != c.endpoint.Host || urlUserString(pageURL) != urlUserString(c.endpoint) {
		return nil, fmt.Errorf("URL %q is not on the GitHub API origin %q", pageURL, c.endpoint)
	}
	if normalizedReleasePath(pageURL.Path) != normalizedReleasePath(c.endpoint.Path) {
		return nil, fmt.Errorf("URL %q path %q is not the GitHub Releases endpoint path %q", pageURL, normalizedReleasePath(pageURL.Path), normalizedReleasePath(c.endpoint.Path))
	}
	return pageURL, nil
}

func urlUserString(pageURL *url.URL) string {
	if pageURL.User == nil {
		return ""
	}
	return pageURL.User.String()
}

func canonicalReleasePageURL(pageURL *url.URL) string {
	canonical := *pageURL
	canonical.Path = normalizedReleasePath(canonical.Path)
	canonical.RawPath = ""
	canonical.Fragment = ""
	canonical.RawFragment = ""
	return canonical.String()
}

func normalizedReleasePath(value string) string {
	return path.Clean("/" + value)
}

func (c *GitHubReleaseClient) writeCache(cache releaseCache) (err error) {
	if err := ensurePrivateReleaseCacheDirectory(c.cacheDir); err != nil {
		return err
	}
	cacheFile, err := os.CreateTemp(c.cacheDir, ".github-releases-*")
	if err != nil {
		return fmt.Errorf("create temporary GitHub Releases cache in %q: %w", c.cacheDir, err)
	}
	temporaryPath := cacheFile.Name()
	defer func() {
		_ = cacheFile.Close()
		if removeErr := os.Remove(temporaryPath); err == nil && removeErr != nil && !os.IsNotExist(removeErr) {
			err = fmt.Errorf("remove temporary GitHub Releases cache %q: %w", temporaryPath, removeErr)
		}
	}()
	if err := cacheFile.Chmod(0o600); err != nil {
		return fmt.Errorf("set temporary GitHub Releases cache permissions %q: %w", temporaryPath, err)
	}
	if err := json.NewEncoder(cacheFile).Encode(cache); err != nil {
		return fmt.Errorf("encode GitHub Releases cache %q: %w", temporaryPath, err)
	}
	if err := cacheFile.Sync(); err != nil {
		return fmt.Errorf("sync temporary GitHub Releases cache %q: %w", temporaryPath, err)
	}
	if err := cacheFile.Close(); err != nil {
		return fmt.Errorf("close temporary GitHub Releases cache %q: %w", temporaryPath, err)
	}
	// ReplaceFile 在 Windows 对已有目标使用 ReplaceFileW，首次创建才用不覆盖的 MoveFileEx。
	if err := files.ReplaceFile(temporaryPath, c.cachePath); err != nil {
		return fmt.Errorf("atomically replace GitHub Releases cache %q: %w", c.cachePath, err)
	}
	return nil
}

func publishedReleases(releases []githubRelease) []githubRelease {
	published := make([]githubRelease, 0, len(releases))
	for _, release := range releases {
		if !release.Draft {
			published = append(published, release)
		}
	}
	return published
}

func selectRelease(releases []githubRelease, includePrerelease bool) (*Release, error) {
	var selected *Release
	var selectedVersion semanticVersion
	for _, candidate := range releases {
		if candidate.Draft {
			continue
		}
		// GitHub 已标记的预发布不属于 stable 渠道，其 tag 可不遵循本项目的 SemVer 发布约定。
		if candidate.Prerelease && !includePrerelease {
			continue
		}
		version, err := parseSemanticVersion(candidate.TagName)
		if err != nil {
			return nil, fmt.Errorf("parse GitHub release tag %q: %w", candidate.TagName, err)
		}
		prerelease := candidate.Prerelease || version.isPrerelease()
		if prerelease && !includePrerelease {
			continue
		}
		if selected == nil || version.compare(selectedVersion) > 0 {
			selectedVersion = version
			selected = &Release{
				TagName:    candidate.TagName,
				Version:    version.string(),
				Prerelease: prerelease,
				Assets:     append([]ReleaseAsset(nil), candidate.Assets...),
			}
		}
	}
	return selected, nil
}

type semanticVersion struct {
	major      string
	minor      string
	patch      string
	prerelease []string
	build      []string
}

func parseSemanticVersion(tag string) (semanticVersion, error) {
	if !strings.HasPrefix(tag, "v") {
		return semanticVersion{}, fmt.Errorf("must start with v")
	}
	versionText := strings.TrimPrefix(tag, "v")
	if versionText == "" {
		return semanticVersion{}, fmt.Errorf("missing version")
	}
	mainAndPre, build, hasBuild := strings.Cut(versionText, "+")
	if hasBuild && !validSemanticIdentifiers(build, false) {
		return semanticVersion{}, fmt.Errorf("invalid build metadata %q", build)
	}
	main, prerelease, hasPrerelease := strings.Cut(mainAndPre, "-")
	if hasPrerelease && !validSemanticIdentifiers(prerelease, true) {
		return semanticVersion{}, fmt.Errorf("invalid prerelease %q", prerelease)
	}
	parts := strings.Split(main, ".")
	if len(parts) != 3 {
		return semanticVersion{}, fmt.Errorf("must contain major.minor.patch")
	}
	major, err := parseSemanticNumber(parts[0])
	if err != nil {
		return semanticVersion{}, fmt.Errorf("invalid major version: %w", err)
	}
	minor, err := parseSemanticNumber(parts[1])
	if err != nil {
		return semanticVersion{}, fmt.Errorf("invalid minor version: %w", err)
	}
	patch, err := parseSemanticNumber(parts[2])
	if err != nil {
		return semanticVersion{}, fmt.Errorf("invalid patch version: %w", err)
	}
	parsed := semanticVersion{major: major, minor: minor, patch: patch}
	if hasPrerelease {
		parsed.prerelease = strings.Split(prerelease, ".")
	}
	if hasBuild {
		parsed.build = strings.Split(build, ".")
	}
	return parsed, nil
}

func parseSemanticNumber(value string) (string, error) {
	if !isNumericIdentifier(value) || (len(value) > 1 && value[0] == '0') {
		return "", fmt.Errorf("%q is not a canonical numeric identifier", value)
	}
	return value, nil
}

func validSemanticIdentifiers(value string, forbidLeadingZeroNumber bool) bool {
	if value == "" {
		return false
	}
	for _, identifier := range strings.Split(value, ".") {
		if identifier == "" {
			return false
		}
		for _, character := range identifier {
			if !((character >= '0' && character <= '9') || (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || character == '-') {
				return false
			}
		}
		if forbidLeadingZeroNumber && isNumericIdentifier(identifier) && len(identifier) > 1 && identifier[0] == '0' {
			return false
		}
	}
	return true
}

func (v semanticVersion) isPrerelease() bool {
	return len(v.prerelease) > 0
}

func (v semanticVersion) string() string {
	value := v.major + "." + v.minor + "." + v.patch
	if v.isPrerelease() {
		value += "-" + strings.Join(v.prerelease, ".")
	}
	if len(v.build) > 0 {
		value += "+" + strings.Join(v.build, ".")
	}
	return value
}

func (v semanticVersion) compare(other semanticVersion) int {
	if compared := compareSemanticNumber(v.major, other.major); compared != 0 {
		return compared
	}
	if compared := compareSemanticNumber(v.minor, other.minor); compared != 0 {
		return compared
	}
	if compared := compareSemanticNumber(v.patch, other.patch); compared != 0 {
		return compared
	}
	if !v.isPrerelease() && !other.isPrerelease() {
		return 0
	}
	if !v.isPrerelease() {
		return 1
	}
	if !other.isPrerelease() {
		return -1
	}
	for index := 0; index < len(v.prerelease) && index < len(other.prerelease); index++ {
		if compared := compareSemanticIdentifier(v.prerelease[index], other.prerelease[index]); compared != 0 {
			return compared
		}
	}
	return compareInt(len(v.prerelease), len(other.prerelease))
}

func compareSemanticIdentifier(first, second string) int {
	firstNumeric := isNumericIdentifier(first)
	secondNumeric := isNumericIdentifier(second)
	if firstNumeric && secondNumeric {
		return compareSemanticNumber(first, second)
	}
	if firstNumeric {
		return -1
	}
	if secondNumeric {
		return 1
	}
	return strings.Compare(first, second)
}

func isNumericIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

// compareSemanticNumber 比较无界、已校验且无前导零的 SemVer 数字标识符。
func compareSemanticNumber(first, second string) int {
	if len(first) < len(second) {
		return -1
	}
	if len(first) > len(second) {
		return 1
	}
	return strings.Compare(first, second)
}

func compareInt(first, second int) int {
	if first < second {
		return -1
	}
	if first > second {
		return 1
	}
	return 0
}
