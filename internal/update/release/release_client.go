package release

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/update/source"
)

const (
	defaultGitHubAPIBaseURL = "https://api.github.com"
	defaultGitHubRepository = "FlanChanXwO/pixiv-cli"
	// CacheFilename 是 release cache 使用的固定文件名。
	CacheFilename = "github-releases.json"
	// releaseCacheSchemaVersion 2 起缓存页保留 Release assets；旧 schema 不能接受 304，
	// 否则显式 self-update 会永久看到缺少资产，直到用户手动删除缓存。
	releaseCacheSchemaVersion = 2
	automaticCheckTimeout     = 3 * time.Second
)

// ReleaseClientOptions 配置 GitHub Releases 客户端。
// APIBaseURL、Repository、HTTPClient、CacheDir 和 Now 允许调用方为受控环境注入依赖；
// Cache 是 release cache 的存储端口，生产默认文件实现由安装器提供。
type ReleaseClientOptions struct {
	APIBaseURL                 string
	Repository                 string
	HTTPClient                 *http.Client
	CacheDir                   string
	Cache                      ReleaseCache
	Now                        func() time.Time
	EnablePublicReleaseSources bool
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

// ReleaseChecker 查询某个更新渠道可用的 GitHub Release。
type ReleaseChecker interface {
	Check(context.Context, ReleaseCheckOptions) (ReleaseCheckResult, error)
}

// GitHubReleaseClient 通过 GitHub Releases API 查询候选版本。
type GitHubReleaseClient struct {
	endpoint   *url.URL
	httpClient *http.Client
	cache      ReleaseCache
	now        func() time.Time
	// sourceSelector 仅在调用方未显式配置 HTTP(S) proxy 时启用，避免改变用户指定代理的语义。
	sourceSelector *source.ReleaseSourceSelector
}

// NewGitHubReleaseClient 建立唯一使用 GitHub Releases API 的更新查询客户端。
func NewGitHubReleaseClient(options ReleaseClientOptions) (*GitHubReleaseClient, error) {
	if options.Cache == nil {
		return nil, fmt.Errorf("release cache is required")
	}
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
	now := options.Now
	if now == nil {
		now = time.Now
	}
	client := &GitHubReleaseClient{
		endpoint:   endpoint,
		httpClient: httpClient,
		cache:      options.Cache,
		now:        now,
	}
	if options.EnablePublicReleaseSources {
		client.sourceSelector = source.NewDefaultReleaseSourceSelector(httpClient)
	}
	return client, nil
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
	if err := source.CheckContext(ctx, "read GitHub Releases cache"); err != nil {
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
	if err := source.CheckContext(ctx, "fetch GitHub Releases pages"); err != nil {
		return ReleaseCheckResult{}, err
	}
	selected, err := selectRelease(refreshedCache.Releases, options.IncludePrerelease)
	if err != nil {
		return ReleaseCheckResult{}, err
	}
	refreshedCache.CheckedAt = now
	if err := c.writeCache(ctx, refreshedCache); err != nil {
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
	if err := source.CheckContext(ctx, "read GitHub Releases cache"); err != nil {
		return releaseCache{}, false, err
	}
	cacheBytes, exists, err := c.cache.Read(ctx)
	if err != nil {
		return releaseCache{}, false, err
	}
	if !exists {
		return releaseCache{}, false, nil
	}
	var cache releaseCache
	if err := json.Unmarshal(cacheBytes, &cache); err != nil {
		return releaseCache{}, false, fmt.Errorf("decode GitHub Releases cache: %w", err)
	}
	if cache.CheckedAt.IsZero() {
		return releaseCache{}, false, fmt.Errorf("decode GitHub Releases cache: missing checked_at")
	}
	if err := source.CheckContext(ctx, "read GitHub Releases cache"); err != nil {
		return releaseCache{}, false, err
	}
	return cache, true, nil
}

// writeCache 把经过版本选择的 cache 交给存储端口原子落盘。
func (c *GitHubReleaseClient) writeCache(ctx context.Context, cache releaseCache) (err error) {
	body, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("encode GitHub Releases cache: %w", err)
	}
	if err := source.CheckContext(ctx, "write GitHub Releases cache"); err != nil {
		return err
	}
	return c.cache.Write(ctx, append(body, '\n'))
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
	var selectedVersion SemanticVersion
	for _, candidate := range releases {
		if candidate.Draft {
			continue
		}
		// GitHub 已标记的预发布不属于 stable 渠道，其 tag 可不遵循本项目的 SemVer 发布约定。
		if candidate.Prerelease && !includePrerelease {
			continue
		}
		// 受信发布 workflow 会对当前通道中的 published tag 强制校验 SemVer；若仍出现
		// 非法 tag，说明发布入口被绕过或 policy 漂移，必须显式失败而不能选择更旧版本。
		version, err := ParseSemanticVersion(candidate.TagName)
		if err != nil {
			return nil, fmt.Errorf("parse GitHub release tag %q: %w", candidate.TagName, err)
		}
		prerelease := candidate.Prerelease || version.IsPrerelease()
		if prerelease && !includePrerelease {
			continue
		}
		if selected == nil || version.Compare(selectedVersion) > 0 {
			selectedVersion = version
			selected = &Release{
				TagName:    candidate.TagName,
				Version:    version.String(),
				Prerelease: prerelease,
				Assets:     append([]ReleaseAsset(nil), candidate.Assets...),
			}
		}
	}
	return selected, nil
}

// ValidateOfficialGitHubReleaseAssetURL 只接受固定仓库、选中 tag 和精确 asset 名组成的
// GitHub HTTPS 下载端点。完整字符串相等也拒绝 query、fragment、userinfo、port 与任何
// encoded 或歧义 path；真正下载时仍交由 HTTP client 跟随 GitHub 的合法重定向。
func ValidateOfficialGitHubReleaseAssetURL(release Release, asset ReleaseAsset) error {
	expected := "https://github.com/" + defaultGitHubRepository + "/releases/download/" + release.TagName + "/" + asset.Name
	if asset.DownloadURL != expected {
		return fmt.Errorf("release asset %q has untrusted download URL %q: expected exact GitHub HTTPS release URL %q", asset.Name, asset.DownloadURL, expected)
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

	var apiSources []source.ReleaseSource
	if c.sourceSelector != nil {
		var err error
		apiSources, err = c.sourceSelector.Ordered(ctx, source.ReleaseSourceAPI, c.endpoint.String())
		if err != nil {
			return releaseCache{}, err
		}
	}
	current := *c.endpoint
	visited := make(map[string]struct{})
	refreshed := releaseCache{SchemaVersion: releaseCacheSchemaVersion}
	for {
		if err := source.CheckContext(ctx, "fetch GitHub Releases page"); err != nil {
			return releaseCache{}, err
		}
		pageID := canonicalReleasePageURL(&current)
		if _, seen := visited[pageID]; seen {
			return releaseCache{}, fmt.Errorf("GitHub Releases pagination loop at %q", pageID)
		}
		visited[pageID] = struct{}{}

		cachedPage, hasCachedPage := cachedPages[pageID]
		page, nextPage, err := c.fetchReleasePage(ctx, &current, cachedPage, hasCachedPage, source.FirstReleaseSource(apiSources))
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

func (c *GitHubReleaseClient) fetchReleasePage(ctx context.Context, pageURL *url.URL, cachedPage releaseCachePage, hasCachedPage bool, selectedSource *source.ReleaseSource) (releaseCachePage, *url.URL, error) {
	requestURL := pageURL.String()
	if selectedSource != nil {
		var err error
		requestURL, err = selectedSource.APIURL(requestURL)
		if err != nil {
			return releaseCachePage{}, nil, fmt.Errorf("transform GitHub Releases page %q through source %q: %w", pageURL, selectedSource.ID(), err)
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return releaseCachePage{}, nil, fmt.Errorf("create GitHub Releases request: %w", err)
	}
	request.Header.Set("User-Agent", source.GitHubUserAgent)
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
	if err := source.CheckContext(ctx, "decode GitHub Releases page"); err != nil {
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
