// Package ascii2d 实现 ascii2d 的浏览器会话、图片上传与 HTML 结果协议。
package ascii2d

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"

	reversesearch "github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
)

const (
	defaultEndpoint = "https://ascii2d.net"
	// MaxImageBytes 来自 ascii2d 官方说明中的单图 10 MB 上限，只作用于该 provider。
	MaxImageBytes int64 = 10 * 1024 * 1024
)

var (
	uploadHashPattern      = regexp.MustCompile(`^[0-9a-f]{32}$`)
	errCrossOriginRedirect = errors.New("ascii2d cross-origin redirect")
	errChallengeDetected   = errors.New("ascii2d challenge detected")
)

func mapSolverError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if reversesearch.CodeOf(err) != reversesearch.CodeUnknown {
		return err
	}
	switch {
	case errors.Is(err, ErrSolverUnavailable):
		return reversesearch.NewError(reversesearch.CodeSolverUnavailable, "ascii2d challenge solver is unavailable", err)
	case errors.Is(err, ErrSolverFailed):
		return reversesearch.NewError(reversesearch.CodeSolverFailed, "ascii2d challenge solver failed", err)
	case errors.Is(err, ErrMalformedSolverResponse):
		return reversesearch.NewError(reversesearch.CodeMalformedSolverResponse, "ascii2d challenge solver response is malformed", err)
	default:
		return err
	}
}

// Options 是 ascii2d adapter 的构造依赖。
type Options struct {
	HTTPClient   *http.Client
	Endpoint     string
	ProxyURL     string
	UserAgent    string
	FlareSolverr *FlareSolverrOptions
}

type clientLifecycle struct {
	once sync.Once
	err  error
}

// Client 持有 ascii2d 会话模板；每次 Upload 都创建独立 cookie jar。
type Client struct {
	httpClient  *http.Client
	endpoint    *url.URL
	userAgent   string
	hints       clientHints
	solver      *solverClient
	solverCache *solverStateCache
	lifecycle   *clientLifecycle
}

// Session 是一次成功上传产生的不可变查询句柄。color 与 bovw 可共享它。
type Session struct {
	client *Client
	hash   string
}

var _ reversesearch.Closer = (*Client)(nil)

func New(options Options) (*Client, error) {
	endpoint := options.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		return nil, reversesearch.NewError(reversesearch.CodeInvalidRequest, "ascii2d endpoint is invalid", nil)
	}
	userAgent, hints, err := normalizeUserAgent(options.UserAgent)
	if err != nil {
		return nil, err
	}
	proxyURL, err := validateProxyURL(options.ProxyURL)
	if err != nil {
		return nil, err
	}
	baseClient := options.HTTPClient
	if baseClient == nil {
		transport, err := newBrowserTransport(proxyURL)
		if err != nil {
			return nil, reversesearch.NewError(reversesearch.CodeProviderFailed, "could not create ascii2d browser transport", nil)
		}
		baseClient = &http.Client{Transport: transport}
	}
	client := *baseClient
	client.Jar = nil
	var solver *solverClient
	if options.FlareSolverr != nil {
		solver, err = newSolverClient(solverClientOptions{FlareSolverr: *options.FlareSolverr})
		if err != nil {
			return nil, mapSolverError(err)
		}
	}
	return &Client{
		httpClient:  &client,
		endpoint:    parsed,
		userAgent:   userAgent,
		hints:       hints,
		solver:      solver,
		solverCache: newSolverStateCache(),
		lifecycle:   &clientLifecycle{},
	}, nil
}

// Close 结束 ascii2d 的进程级 solver session，并释放 native/browser client
// 的空闲连接。session client 与模板 client 共享同一 lifecycle，因此重复
// Close 或从任一 session 关闭都不会重复 destroy 同一个 solver session。
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	if c.lifecycle == nil {
		// 只有内部零值/旧测试构造会缺少 lifecycle；正常实例均由 New 初始化。
		c.lifecycle = &clientLifecycle{}
	}
	c.lifecycle.once.Do(func() {
		if c.solverCache != nil {
			c.solverCache.close()
		}
		if c.solver != nil {
			c.lifecycle.err = errors.Join(c.lifecycle.err, c.solver.destroy(context.Background()))
			if c.solver.httpClient != nil {
				c.solver.httpClient.CloseIdleConnections()
			}
		}
		if c.httpClient != nil {
			c.httpClient.CloseIdleConnections()
		}
	})
	return c.lifecycle.err
}

func (c *Client) Preflight(ctx context.Context) error {
	if ctx == nil {
		return reversesearch.NewError(reversesearch.CodeInvalidRequest, "reverse search context is required", nil)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if c == nil || c.httpClient == nil || c.endpoint == nil {
		return reversesearch.NewError(reversesearch.CodeProviderNotConfigured, "ascii2d provider is not configured", nil)
	}
	return nil
}

// Upload 校验 ascii2d 专属媒体约束；每次上传尝试都从 Snapshot 重新打开并构建 multipart。
func (c *Client) Upload(ctx context.Context, snapshot *reversesearch.Snapshot) (reversesearch.ASCII2DSession, error) {
	if err := c.Preflight(ctx); err != nil {
		return nil, err
	}
	mediaType, err := validateSnapshot(snapshot)
	if err != nil {
		return nil, err
	}
	sessionClient, err := c.newSessionClient()
	if err != nil {
		return nil, err
	}
	token, err := sessionClient.fetchCSRF(ctx)
	if err != nil {
		if errors.Is(err, errChallengeDetected) {
			return c.recoverUpload(ctx, snapshot, mediaType, sessionClient.endpoint.String(), err)
		}
		return nil, err
	}
	hash, err := sessionClient.upload(ctx, snapshot, mediaType, token)
	if err != nil {
		if errors.Is(err, errChallengeDetected) {
			return c.recoverUpload(ctx, snapshot, mediaType, sessionClient.resolvePath("/search/file").String(), err)
		}
		return nil, err
	}
	return &Session{client: sessionClient, hash: hash}, nil
}

func (c *Client) recoverUpload(ctx context.Context, snapshot *reversesearch.Snapshot, mediaType, protectedURL string, challengeErr error) (reversesearch.ASCII2DSession, error) {
	if c == nil || c.solver == nil {
		return nil, reversesearch.NewError(reversesearch.CodeChallengeRequired, "ascii2d challenge requires solver recovery", challengeErr)
	}
	// challenge 只允许一次恢复重放；恢复后的 session/CSRF 与原尝试完全隔离。
	state, err := c.solveChallenge(ctx, protectedURL)
	if err != nil {
		return nil, mapSolverError(err)
	}
	sessionClient, err := c.newSessionClientWithSolverState(state)
	if err != nil {
		c.solverCache.invalidate()
		return nil, mapSolverError(err)
	}
	token, err := sessionClient.fetchCSRF(ctx)
	if err != nil {
		if errors.Is(err, errChallengeDetected) {
			c.solverCache.invalidate()
			return nil, mapSolverError(ErrSolverFailed)
		}
		return nil, err
	}
	hash, err := sessionClient.upload(ctx, snapshot, mediaType, token)
	if err != nil {
		if errors.Is(err, errChallengeDetected) {
			c.solverCache.invalidate()
			return nil, mapSolverError(ErrSolverFailed)
		}
		return nil, err
	}
	return &Session{client: sessionClient, hash: hash}, nil
}

func (c *Client) solveChallenge(ctx context.Context, protectedURL string) (solverState, error) {
	if c == nil || c.solver == nil || c.solverCache == nil || c.endpoint == nil {
		return solverState{}, mapSolverError(ErrSolverUnavailable)
	}
	state, err := c.solverCache.getOrSolve(ctx, func(solveContext context.Context) (solverState, error) {
		if err := c.solver.create(solveContext); err != nil {
			return solverState{}, err
		}
		return c.solver.get(solveContext, protectedURL)
	})
	return state, mapSolverError(err)
}

func (c *Client) newSessionClient() (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, reversesearch.NewError(reversesearch.CodeProviderFailed, "could not create ascii2d session", nil)
	}
	httpClient := *c.httpClient
	httpClient.Jar = jar
	originalRedirect := httpClient.CheckRedirect
	httpClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if !sameOrigin(c.endpoint, next.URL) {
			return errCrossOriginRedirect
		}
		if originalRedirect != nil {
			return originalRedirect(next, via)
		}
		if len(via) >= 10 {
			// 保留 net/http 的默认十跳规则，不为 ascii2d 新增自定义重试或阈值。
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &Client{
		httpClient:  &httpClient,
		endpoint:    c.endpoint,
		userAgent:   c.userAgent,
		hints:       c.hints,
		solver:      c.solver,
		solverCache: c.solverCache,
		lifecycle:   c.lifecycle,
	}, nil
}

func (c *Client) newSessionClientWithSolverState(state solverState) (*Client, error) {
	if !validSolverHeaderValue(state.userAgent) || state.clearance == "" || !validSolverCookieValue(state.clearance) {
		return nil, ErrMalformedSolverResponse
	}
	userAgent, hints, err := normalizeUserAgent(state.userAgent)
	if err != nil {
		return nil, ErrMalformedSolverResponse
	}
	sessionClient, err := c.newSessionClient()
	if err != nil {
		return nil, err
	}
	sessionClient.userAgent = userAgent
	sessionClient.hints = hints
	sessionClient.httpClient.Jar.SetCookies(sessionClient.endpoint, []*http.Cookie{{
		Name:  "cf_clearance",
		Value: state.clearance,
		Path:  "/",
	}})
	return sessionClient, nil
}

func validateSnapshot(snapshot *reversesearch.Snapshot) (string, error) {
	if snapshot == nil {
		return "", reversesearch.NewError(reversesearch.CodeInvalidRequest, "image snapshot is required", nil)
	}
	if snapshot.Size() > MaxImageBytes {
		return "", reversesearch.NewError(reversesearch.CodeInvalidSource, "ascii2d image exceeds the 10 MB limit", nil)
	}
	reader, err := snapshot.Open()
	if err != nil {
		return "", err
	}
	defer reader.Close()
	buffer := make([]byte, 512)
	read, readErr := io.ReadFull(reader, buffer)
	// io.ReadFull 在 0 字节输入时返回 io.EOF（而非 io.ErrUnexpectedEOF）。
	// 空图片是用户输入问题，应归类为 CodeInvalidSource，否则调用方会误判为本地内部故障。
	if errors.Is(readErr, io.EOF) {
		return "", reversesearch.NewError(reversesearch.CodeInvalidSource, "ascii2d supports only JPEG, PNG, or WEBP images", nil)
	}
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return "", reversesearch.NewError(reversesearch.CodeSnapshotFailed, "could not inspect image snapshot", nil)
	}
	mediaType := http.DetectContentType(buffer[:read])
	switch mediaType {
	case "image/jpeg", "image/png", "image/webp":
		return mediaType, nil
	default:
		return "", reversesearch.NewError(reversesearch.CodeInvalidSource, "ascii2d supports only JPEG, PNG, or WEBP images", nil)
	}
}

func (c *Client) fetchCSRF(ctx context.Context) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint.String(), nil)
	if err != nil {
		return "", reversesearch.NewError(reversesearch.CodeProviderFailed, "could not create ascii2d session request", nil)
	}
	c.applyNavigationHeaders(request, "none", "")
	response, err := c.httpClient.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		if errors.Is(err, errCrossOriginRedirect) {
			return "", reversesearch.NewError(reversesearch.CodeMalformedUpstreamResponse, "ascii2d redirected outside its origin", nil)
		}
		return "", reversesearch.NewError(reversesearch.CodeProviderFailed, "ascii2d session request failed", nil)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		if challengeErr := detectResponseChallenge(ctx, response); challengeErr != nil {
			return "", challengeErr
		}
	} else {
		return "", classifyResponseError(ctx, response, "ascii2d returned an unsuccessful HTTP status")
	}
	token, parseErr := parseUploadForm(response.Body)
	if parseErr != nil {
		return "", reversesearch.NewError(reversesearch.CodeMalformedUpstreamResponse, "ascii2d returned a malformed response", nil)
	}
	return token, nil
}

// classifyResponseError 先处理 Cloudflare 的权威响应头，再按需流式扫描 403 HTML。
// 普通上游错误仍保留原有分类；challenge 的 cause 只供 provider 内部后续恢复，
// 不把上游响应正文带入跨边界错误文本。
func (c *Client) resolvePath(value string) *url.URL {
	reference := &url.URL{Path: value}
	return c.endpoint.ResolveReference(reference)
}

// Search 获取 Session 对应的 color 或 bovw 页面并解析结果。
func (s *Session) Search(ctx context.Context, provider reversesearch.Provider) (reversesearch.ProviderResponse, error) {
	if ctx == nil {
		return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeInvalidRequest, "reverse search context is required", nil)
	}
	if err := ctx.Err(); err != nil {
		return reversesearch.ProviderResponse{}, err
	}
	if s == nil || s.client == nil || !uploadHashPattern.MatchString(s.hash) {
		return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeProviderNotConfigured, "ascii2d session is not configured", nil)
	}
	mode := ""
	switch provider {
	case reversesearch.ProviderASCII2DColor:
		mode = "color"
	case reversesearch.ProviderASCII2DBOVW:
		mode = "bovw"
	default:
		return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeInvalidRequest, "ascii2d search mode is invalid", nil)
	}
	resultURL := s.client.resolvePath(fmt.Sprintf("/search/%s/%s", mode, s.hash))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, resultURL.String(), nil)
	if err != nil {
		return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeProviderFailed, "could not create ascii2d result request", nil)
	}
	s.client.applyNavigationHeaders(request, "same-origin", s.client.homeURL())
	response, err := s.client.httpClient.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return reversesearch.ProviderResponse{}, ctxErr
		}
		if errors.Is(err, errCrossOriginRedirect) {
			return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeMalformedUpstreamResponse, "ascii2d redirected outside its origin", nil)
		}
		return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeProviderFailed, "ascii2d result request failed", nil)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		if challengeErr := detectResponseChallenge(ctx, response); challengeErr != nil {
			return reversesearch.ProviderResponse{}, challengeErr
		}
	} else {
		return reversesearch.ProviderResponse{}, classifyResponseError(ctx, response, "ascii2d returned an unsuccessful HTTP status")
	}
	matches, parseErr := parseResults(response.Body)
	if parseErr != nil {
		return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeMalformedUpstreamResponse, "ascii2d returned a malformed result page", nil)
	}
	return reversesearch.ProviderResponse{Provider: provider, Matches: matches}, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
