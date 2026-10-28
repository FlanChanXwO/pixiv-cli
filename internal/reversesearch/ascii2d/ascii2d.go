// Package ascii2d 实现 ascii2d 的浏览器会话、图片上传与 HTML 结果协议。
//
// 包的组成：[New] 构造 [Client]，[Client] 完成上传并通过 [Session] 查询 color 与
// bovw 两个入口；transport.go 提供浏览器指纹，solver.go 与 challenge.go 处理
// Cloudflare challenge，html.go 解析结果页。
package ascii2d

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sync"

	reversesearch "github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
)

// maxImageBytes 来自 ascii2d 官方说明中的单图 10 MB 上限，只作用于该 provider。
const maxImageBytes int64 = 10 * 1024 * 1024

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
	case errors.Is(err, errSolverUnavailable):
		return reversesearch.NewError(reversesearch.CodeSolverUnavailable, "ascii2d challenge solver is unavailable", err)
	case errors.Is(err, errSolverFailed):
		return reversesearch.NewError(reversesearch.CodeSolverFailed, "ascii2d challenge solver failed", err)
	case errors.Is(err, errMalformedSolverResponse):
		return reversesearch.NewError(reversesearch.CodeMalformedSolverResponse, "ascii2d challenge solver response is malformed", err)
	default:
		return err
	}
}

// Options 提供 ascii2d adapter 的构造依赖。
type Options struct {
	ProxyURL  string
	UserAgent string
	// FlareSolverrURL 为 nil 时关闭 recovery；显式空地址仍按无效配置报错。
	FlareSolverrURL *string
	// FlareSolverrProxyURL 只用于 solver 的 browser upstream；control 请求直连。
	FlareSolverrProxyURL string
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

// Session 持有一次成功上传产生的不可变查询句柄。color 与 bovw 可共享它。
type Session struct {
	client *Client
	hash   string
}

var _ reversesearch.Closer = (*Client)(nil)

// New 构造 ascii2d client；传输、代理与可选 solver 配置都来自 options。
func New(options Options) (*Client, error) {
	userAgent, hints, err := normalizeUserAgent(options.UserAgent)
	if err != nil {
		return nil, err
	}
	proxyURL, err := validateProxyURL(options.ProxyURL)
	if err != nil {
		return nil, err
	}
	transport, err := newBrowserTransport(proxyURL)
	if err != nil {
		return nil, reversesearch.NewError(reversesearch.CodeProviderFailed, "could not create ascii2d browser transport", err)
	}
	client := &http.Client{Transport: transport}
	var solver *solverClient
	if options.FlareSolverrURL != nil {
		solver, err = newSolverClient(*options.FlareSolverrURL, options.FlareSolverrProxyURL)
		if err != nil {
			return nil, mapSolverError(err)
		}
	}
	return &Client{
		httpClient:  client,
		endpoint:    &url.URL{Scheme: "https", Host: "ascii2d.net"},
		userAgent:   userAgent,
		hints:       hints,
		solver:      solver,
		solverCache: &solverStateCache{},
		lifecycle:   &clientLifecycle{},
	}, nil
}

// Close 结束 ascii2d 的进程级 solver session，并释放 native/browser client
// 的空闲连接；模板 client 负责关闭会话共用的资源，重复 Close 保持幂等。
func (c *Client) Close() error {
	c.lifecycle.once.Do(func() {
		c.solverCache.close()
		if c.solver != nil {
			c.lifecycle.err = mapSolverError(c.solver.destroy(context.Background()))
			c.solver.httpClient.CloseIdleConnections()
		}
		c.httpClient.CloseIdleConnections()
	})
	return c.lifecycle.err
}

// Upload 校验 ascii2d 专属媒体约束；每次上传尝试都从 Snapshot 重新打开并构建 multipart。
func (c *Client) Upload(ctx context.Context, snapshot *reversesearch.Snapshot) (reversesearch.ASCII2DSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	mediaType, err := validateSnapshot(ctx, snapshot)
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
	if c.solver == nil {
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
			return nil, mapSolverError(errSolverFailed)
		}
		return nil, err
	}
	hash, err := sessionClient.upload(ctx, snapshot, mediaType, token)
	if err != nil {
		if errors.Is(err, errChallengeDetected) {
			c.solverCache.invalidate()
			return nil, mapSolverError(errSolverFailed)
		}
		return nil, err
	}
	return &Session{client: sessionClient, hash: hash}, nil
}

func (c *Client) solveChallenge(ctx context.Context, protectedURL string) (solverState, error) {
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
		return nil, reversesearch.NewError(reversesearch.CodeProviderFailed, "could not create ascii2d session", err)
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
	sessionClient, err := c.newSessionClient()
	if err != nil {
		return nil, err
	}
	sessionClient.userAgent = state.userAgent
	sessionClient.hints = parseClientHints(state.userAgent)
	sessionClient.httpClient.Jar.SetCookies(sessionClient.endpoint, []*http.Cookie{{
		Name:  "cf_clearance",
		Value: state.clearance,
		Path:  "/",
	}})
	return sessionClient, nil
}

func validateSnapshot(ctx context.Context, snapshot *reversesearch.Snapshot) (string, error) {
	if snapshot.Size() > maxImageBytes {
		return "", reversesearch.NewError(reversesearch.CodeInvalidSource, "ascii2d image exceeds the 10 MB limit", nil)
	}
	reader, err := snapshot.Open(ctx)
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
		return "", reversesearch.NewError(reversesearch.CodeSnapshotFailed, "could not inspect image snapshot", readErr)
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
		return "", reversesearch.NewError(reversesearch.CodeProviderFailed, "could not create ascii2d session request", err)
	}
	c.applyNavigationHeaders(request, "none", "")
	response, err := c.httpClient.Do(request)
	if err != nil {
		logging.From(ctx).LogAttrs(ctx, slog.LevelDebug, "Request failed", logging.TransportAttrs(err)...)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		if errors.Is(err, errCrossOriginRedirect) {
			return "", reversesearch.NewError(reversesearch.CodeMalformedUpstreamResponse, "ascii2d redirected outside its origin", err)
		}
		return "", reversesearch.NewError(reversesearch.CodeProviderFailed, "ascii2d session request failed", err)
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
		return "", reversesearch.NewError(reversesearch.CodeMalformedUpstreamResponse, "ascii2d returned a malformed response", parseErr)
	}
	return token, nil
}

func (c *Client) resolvePath(value string) *url.URL {
	reference := &url.URL{Path: value}
	return c.endpoint.ResolveReference(reference)
}

// Search 获取 Session 对应的 color 或 bovw 页面并解析结果。
func (s *Session) Search(ctx context.Context, provider reversesearch.Provider) (reversesearch.ProviderResponse, error) {
	if err := ctx.Err(); err != nil {
		return reversesearch.ProviderResponse{}, err
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
		return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeProviderFailed, "could not create ascii2d result request", err)
	}
	s.client.applyNavigationHeaders(request, "same-origin", s.client.homeURL())
	response, err := s.client.httpClient.Do(request)
	if err != nil {
		logging.From(ctx).LogAttrs(ctx, slog.LevelDebug, "Request failed", logging.TransportAttrs(err)...)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return reversesearch.ProviderResponse{}, ctxErr
		}
		if errors.Is(err, errCrossOriginRedirect) {
			return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeMalformedUpstreamResponse, "ascii2d redirected outside its origin", err)
		}
		return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeProviderFailed, "ascii2d result request failed", err)
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
		return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeMalformedUpstreamResponse, "ascii2d returned a malformed result page", parseErr)
	}
	return reversesearch.ProviderResponse{Matches: matches}, nil
}
