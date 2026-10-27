package ascii2d

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	// errSolverUnavailable 表示无法通过 direct control transport 到达配置的 solver。
	errSolverUnavailable = errors.New("ascii2d: FlareSolverr service unavailable")
	// errSolverFailed 表示 solver 已响应，但没有完成 challenge recovery。
	errSolverFailed = errors.New("ascii2d: FlareSolverr could not solve challenge")
	// errMalformedSolverResponse 表示 solver 响应不满足 clearance-only 状态契约。
	errMalformedSolverResponse = errors.New("ascii2d: malformed FlareSolverr response")
)

const (
	// FlareSolverr 的 maxTimeout 是 request.get 的协议字段；180000 与本目标的
	// 真实验证配置一致。这里不创建独立 Go deadline，调用方 context 负责取消。
	flareSolverrMaxTimeout = 180000
	solverSessionPrefix    = "pixiv-cli-ascii2d-"
)

// solverClient 只负责 FlareSolverr v1 control protocol；它不上传图片，也不接触
// ascii2d 的账号、CSRF 或其他站点 cookie。
type solverClient struct {
	endpoint   string
	proxyURL   string
	httpClient *http.Client
	session    string

	mu      sync.Mutex
	created bool
}

type solverState struct {
	userAgent string
	clearance string
	expiresAt time.Time
	hasExpiry bool
}

type solverRequest struct {
	Command    string       `json:"cmd"`
	Session    string       `json:"session,omitempty"`
	URL        string       `json:"url,omitempty"`
	Proxy      *solverProxy `json:"proxy,omitempty"`
	MaxTimeout int          `json:"maxTimeout,omitempty"`
}

type solverProxy struct {
	URL string `json:"url"`
}

type solverResponse struct {
	Status   string                  `json:"status"`
	Solution *solverResponseSolution `json:"solution"`
}

type solverResponseSolution struct {
	UserAgent string         `json:"userAgent"`
	Cookies   []solverCookie `json:"cookies"`
}

type solverCookie struct {
	Name    string          `json:"name"`
	Value   string          `json:"value"`
	Expires json.RawMessage `json:"expires"`
	Expiry  json.RawMessage `json:"expiry"`
}

func newSolverClient(rawURL, proxyURL string) (*solverClient, error) {
	endpoint, ok := normalizeSolverEndpoint(rawURL)
	if !ok {
		return nil, errSolverUnavailable
	}

	session, err := newSolverSessionName()
	if err != nil {
		return nil, errors.Join(errSolverUnavailable, err)
	}
	// solver control 直连；浏览器代理仅通过 sessions.create 传入。
	httpClient := &http.Client{
		Transport: &http.Transport{Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &solverClient{
		endpoint:   endpoint,
		proxyURL:   proxyURL,
		httpClient: httpClient,
		session:    session,
	}, nil
}

func newSolverSessionName() (string, error) {
	var randomBytes [16]byte
	if _, err := cryptorand.Read(randomBytes[:]); err != nil {
		return "", err
	}
	return solverSessionPrefix + hex.EncodeToString(randomBytes[:]), nil
}

func normalizeSolverEndpoint(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.ForceQuery {
		return "", false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return "", false
	}
	return strings.TrimRight(raw, "/"), true
}

func (c *solverClient) create(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.created {
		return nil
	}
	_, err := c.call(ctx, solverRequest{
		Command: "sessions.create",
		Session: c.session,
		Proxy:   solverProxyForCreate(c.proxyURL),
	})
	if err == nil {
		c.created = true
	}
	return err
}

func solverProxyForCreate(proxyURL string) *solverProxy {
	if proxyURL == "" {
		return nil
	}
	return &solverProxy{URL: proxyURL}
}

func (c *solverClient) get(ctx context.Context, targetURL string) (solverState, error) {
	response, err := c.call(ctx, solverRequest{
		Command:    "request.get",
		Session:    c.session,
		URL:        targetURL,
		MaxTimeout: flareSolverrMaxTimeout,
	})
	if err != nil {
		return solverState{}, err
	}
	return validateSolverSolution(response.Solution)
}

func (c *solverClient) destroy(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.created {
		return nil
	}
	_, err := c.call(ctx, solverRequest{
		Command: "sessions.destroy",
		Session: c.session,
	})
	if err == nil {
		c.created = false
	}
	return err
}

func (c *solverClient) call(ctx context.Context, payload solverRequest) (solverResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return solverResponse{}, errors.Join(errMalformedSolverResponse, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1", bytes.NewReader(body))
	if err != nil {
		return solverResponse{}, errors.Join(errSolverUnavailable, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return solverResponse{}, ctxErr
		}
		return solverResponse{}, errors.Join(errSolverUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return solverResponse{}, errSolverFailed
	}

	decoder := json.NewDecoder(response.Body)
	var decoded solverResponse
	if err := decoder.Decode(&decoded); err != nil {
		return solverResponse{}, errors.Join(errMalformedSolverResponse, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return solverResponse{}, errors.Join(errMalformedSolverResponse, err)
	}
	switch decoded.Status {
	case "ok":
		return decoded, nil
	case "error":
		return solverResponse{}, errSolverFailed
	default:
		return solverResponse{}, errMalformedSolverResponse
	}
}

func validateSolverSolution(solution *solverResponseSolution) (solverState, error) {
	if solution == nil || !validSolverHeaderValue(solution.UserAgent) {
		return solverState{}, errMalformedSolverResponse
	}

	var clearance string
	var expiryRaw json.RawMessage
	count := 0
	for _, cookie := range solution.Cookies {
		if cookie.Name != "cf_clearance" {
			// 只提取 clearance；其他 solver cookie 不进入 ascii2d jar。
			continue
		}
		count++
		if count > 1 || cookie.Value == "" || !validSolverCookieValue(cookie.Value) {
			return solverState{}, errMalformedSolverResponse
		}
		clearance = cookie.Value
		if len(bytes.TrimSpace(cookie.Expiry)) != 0 && !bytes.Equal(bytes.TrimSpace(cookie.Expiry), []byte("null")) {
			expiryRaw = cookie.Expiry
		} else if len(bytes.TrimSpace(cookie.Expires)) != 0 && !bytes.Equal(bytes.TrimSpace(cookie.Expires), []byte("null")) {
			expiryRaw = cookie.Expires
		}
	}
	if count != 1 {
		return solverState{}, errMalformedSolverResponse
	}
	expiresAt, hasExpiry, err := parseSolverExpiry(expiryRaw)
	if err != nil {
		return solverState{}, errMalformedSolverResponse
	}
	return solverState{userAgent: solution.UserAgent, clearance: clearance, expiresAt: expiresAt, hasExpiry: hasExpiry}, nil
}

func validSolverHeaderValue(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}
	return true
}

func validSolverCookieValue(value string) bool {
	for _, char := range value {
		// RFC 6265 cookie-octet 排除控制符、空白、双引号、逗号、分号和反斜杠。
		if char < 0x21 || char > 0x7e || strings.ContainsRune("\",;\\", char) {
			return false
		}
	}
	return true
}

func parseSolverExpiry(raw json.RawMessage) (time.Time, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return time.Time{}, false, nil
	}

	var numeric json.Number
	if err := json.Unmarshal(trimmed, &numeric); err == nil {
		value, err := numeric.Int64()
		if err != nil || value <= 0 {
			return time.Time{}, false, errors.New("solver expiry is invalid")
		}
		return time.Unix(value, 0), true, nil
	}

	var text string
	if err := json.Unmarshal(trimmed, &text); err != nil || text == "" {
		return time.Time{}, false, errors.New("solver expiry is invalid")
	}
	for _, layout := range []string{time.RFC3339, http.TimeFormat} {
		if value, err := time.Parse(layout, text); err == nil {
			return value, true, nil
		}
	}
	return time.Time{}, false, errors.New("solver expiry is invalid")
}
