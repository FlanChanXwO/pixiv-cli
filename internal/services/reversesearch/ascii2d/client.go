// Package ascii2d 实现 ascii2d 的浏览器会话、图片上传与 HTML 结果协议。
package ascii2d

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/textproto"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	reversesearch "github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
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

// Options 是 ascii2d adapter 的构造依赖。
type Options struct {
	HTTPClient *http.Client
	Endpoint   string
	ProxyURL   string
	UserAgent  string
}

// Client 持有 ascii2d 会话模板；每次 Upload 都创建独立 cookie jar。
type Client struct {
	httpClient *http.Client
	endpoint   *url.URL
	userAgent  string
	hints      clientHints
}

// Session 是一次成功上传产生的不可变查询句柄。color 与 bovw 可共享它。
type Session struct {
	client *Client
	hash   string
}

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
	return &Client{httpClient: &client, endpoint: parsed, userAgent: userAgent, hints: hints}, nil
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

// Upload 校验 ascii2d 专属媒体约束，建立 Cookie/CSRF 会话并只上传一次快照。
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
		return nil, err
	}
	hash, err := sessionClient.upload(ctx, snapshot, mediaType, token)
	if err != nil {
		return nil, err
	}
	return &Session{client: sessionClient, hash: hash}, nil
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
		httpClient: &httpClient,
		endpoint:   c.endpoint,
		userAgent:  c.userAgent,
		hints:      c.hints,
	}, nil
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
func classifyResponseError(ctx context.Context, response *http.Response, fallbackMessage string) error {
	if challengeErr := detectResponseChallenge(ctx, response); challengeErr != nil {
		return challengeErr
	}
	return reversesearch.NewError(reversesearch.CodeUpstreamHTTPStatus, fallbackMessage, nil)
}

func detectResponseChallenge(ctx context.Context, response *http.Response) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	challenged, readErr := detectChallenge(response)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if readErr != nil {
		return reversesearch.NewError(reversesearch.CodeProviderFailed, "could not read ascii2d error response", nil)
	}
	if challenged {
		return reversesearch.NewError(reversesearch.CodeUpstreamHTTPStatus, "ascii2d challenge detected", errChallengeDetected)
	}
	return nil
}

func detectChallenge(response *http.Response) (bool, error) {
	if response == nil {
		return false, nil
	}
	// cf-mitigated 是 Cloudflare 明确声明 challenge 的权威信号，优先于正文。
	if challengeHeaderDetected(response.Header) {
		return true, nil
	}
	if response.StatusCode != http.StatusForbidden || response.Body == nil || !isHTMLContentType(response.Header) {
		return false, nil
	}
	return scanHTMLChallenge(response.Body)
}

func challengeHeaderDetected(header http.Header) bool {
	for _, value := range header.Values("Cf-Mitigated") {
		for _, item := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(item), "challenge") {
				return true
			}
		}
	}
	return false
}

func isHTMLContentType(header http.Header) bool {
	mediaType, _, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil {
		return false
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	return mediaType == "text/html" || mediaType == "application/xhtml+xml" || strings.HasSuffix(mediaType, "+html")
}

func scanHTMLChallenge(body io.Reader) (bool, error) {
	tokenizer := html.NewTokenizer(body)
	activeHeading := ""
	var headingText strings.Builder
	for {
		switch tokenType := tokenizer.Next(); tokenType {
		case html.ErrorToken:
			if errors.Is(tokenizer.Err(), io.EOF) {
				return false, nil
			}
			return false, tokenizer.Err()
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			if tokenHasChallengeMarker(token) {
				return true, nil
			}
			if isChallengeHeading(token.Data) {
				activeHeading = token.Data
				headingText.Reset()
			}
		case html.EndTagToken:
			token := tokenizer.Token()
			if token.Data == activeHeading {
				activeHeading = ""
			}
		case html.TextToken, html.CommentToken:
			token := tokenizer.Token()
			text := normalizeChallengeText(token.Data)
			if hasChallengeText(text) {
				return true, nil
			}
			if activeHeading != "" {
				headingText.WriteString(" ")
				headingText.WriteString(text)
				if hasChallengeText(normalizeChallengeText(headingText.String())) {
					return true, nil
				}
			}
		}
	}
}

func tokenHasChallengeMarker(token html.Token) bool {
	for _, attribute := range token.Attr {
		value := strings.ToLower(attribute.Key + "=" + attribute.Val)
		if strings.Contains(value, "cf-chl") || strings.Contains(value, "cf_chl") || strings.Contains(value, "challenge-platform") {
			return true
		}
	}
	return false
}

func isChallengeHeading(name string) bool {
	switch strings.ToLower(name) {
	case "title", "h1", "h2":
		return true
	default:
		return false
	}
}

func normalizeChallengeText(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}

func hasChallengeText(value string) bool {
	// Ray ID、access-denied 和通用 WAF 标题不是 challenge 证据，避免误触发 solver。
	for _, marker := range []string{
		"just a moment",
		"verify you are human",
		"checking your browser before accessing",
		"challenge-platform",
		"cf-chl-",
		"cf_chl",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func (c *Client) upload(ctx context.Context, snapshot *reversesearch.Snapshot, mediaType, token string) (string, error) {
	body, contentType, writeResult := uploadBody(ctx, snapshot, mediaType, token)
	uploadURL := c.resolvePath("/search/file")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL.String(), body)
	if err != nil {
		_ = body.CloseWithError(err)
		<-writeResult
		return "", reversesearch.NewError(reversesearch.CodeProviderFailed, "could not create ascii2d upload request", nil)
	}
	c.applyFormHeaders(request)
	request.Header.Set("Content-Type", contentType)

	client := *c.httpClient
	// 上传必须观察原始 Location；自动跟随会丢失服务端生成的查询 hash。
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		_ = body.CloseWithError(err)
		<-writeResult
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", reversesearch.NewError(reversesearch.CodeProviderFailed, "ascii2d upload request failed", nil)
	}
	defer response.Body.Close()
	writeErr := <-writeResult
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	if response.StatusCode >= http.StatusMultipleChoices && response.StatusCode < http.StatusBadRequest {
		if challengeErr := detectResponseChallenge(ctx, response); challengeErr != nil {
			return "", challengeErr
		}
	} else {
		return "", classifyResponseError(ctx, response, "ascii2d returned an unsuccessful HTTP status")
	}
	hash, locationErr := c.validateUploadLocation(response.Header.Get("Location"))
	if locationErr != nil {
		return "", locationErr
	}
	if writeErr != nil {
		return "", reversesearch.NewError(reversesearch.CodeProviderFailed, "could not upload image to ascii2d", nil)
	}
	return hash, nil
}

func uploadBody(ctx context.Context, snapshot *reversesearch.Snapshot, mediaType, token string) (*io.PipeReader, string, <-chan error) {
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	result := make(chan error, 1)
	go func() {
		err := multipartWriter.WriteField("authenticity_token", token)
		if err == nil {
			filename := "image" + mediaExtension(mediaType)
			header := textproto.MIMEHeader{
				"Content-Disposition": {fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename)},
				"Content-Type":        {mediaType},
			}
			var part io.Writer
			part, err = multipartWriter.CreatePart(header)
			if err == nil {
				var source io.ReadCloser
				source, err = snapshot.Open()
				if err == nil {
					_, err = io.Copy(part, contextReader{ctx: ctx, reader: source})
					if closeErr := source.Close(); err == nil {
						err = closeErr
					}
				}
			}
		}
		if closeErr := multipartWriter.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = writer.CloseWithError(err)
		} else {
			err = writer.Close()
		}
		result <- err
		close(result)
	}()
	return reader, multipartWriter.FormDataContentType(), result
}

func mediaExtension(mediaType string) string {
	switch mediaType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}

func (c *Client) validateUploadLocation(raw string) (string, error) {
	location, err := url.Parse(raw)
	if err != nil || raw == "" || location.User != nil {
		return "", reversesearch.NewError(reversesearch.CodeMalformedUpstreamResponse, "ascii2d returned a malformed upload location", nil)
	}
	resolved := c.endpoint.ResolveReference(location)
	if !sameOrigin(c.endpoint, resolved) {
		return "", reversesearch.NewError(reversesearch.CodeMalformedUpstreamResponse, "ascii2d returned an unsafe upload location", nil)
	}
	if resolved.RawQuery != "" || resolved.Fragment != "" {
		return "", reversesearch.NewError(reversesearch.CodeMalformedUpstreamResponse, "ascii2d returned a malformed upload location", nil)
	}
	segments := strings.Split(strings.TrimPrefix(resolved.EscapedPath(), "/"), "/")
	if len(segments) != 3 || segments[0] != "search" || segments[1] != "color" || !uploadHashPattern.MatchString(segments[2]) {
		return "", reversesearch.NewError(reversesearch.CodeMalformedUpstreamResponse, "ascii2d returned a malformed upload location", nil)
	}
	return segments[2], nil
}

func sameOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Hostname(), right.Hostname()) && effectivePort(left) == effectivePort(right)
}

func effectivePort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	if strings.EqualFold(value.Scheme, "https") {
		return "443"
	}
	return "80"
}

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
