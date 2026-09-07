package stamps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

// Transport 是 stamps read leaf 所需的最小 App API 传输能力。
type Transport interface {
	GetJSON(context.Context, string, url.Values, any) error
}

// Stamp 是 stamps endpoint 当前只归一化的最小实体。
// 其他未被 snapshot 冻结的 wire 字段不进入该内部结果，也不形成 public contract。
type Stamp struct {
	ID  int64
	URL string
}

type Result struct {
	Items []Stamp
}

type Client struct{ transport Transport }

func New(transport Transport) *Client { return &Client{transport: transport} }

func (c *Client) List(ctx context.Context) (Result, error) {
	if c == nil || c.transport == nil {
		return Result{}, errors.New("stamps transport is not configured")
	}

	var raw responseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppStamps, nil, &raw); err != nil {
		return Result{}, err
	}
	if !raw.Stamps.Present || !raw.Stamps.Valid || raw.NextURL != nil {
		return Result{}, protocol.MalformedResponse()
	}

	items := make([]Stamp, len(raw.Stamps.Items))
	for index, value := range raw.Stamps.Items {
		if value.ID <= 0 || !validResourceURL(value.URL) {
			return Result{}, protocol.MalformedResponse()
		}
		items[index] = Stamp{ID: value.ID, URL: value.URL}
	}
	return Result{Items: items}, nil
}

type responseDTO struct {
	Stamps  requiredList[stampDTO] `json:"stamps"`
	NextURL *json.RawMessage       `json:"next_url"`
}

type stampDTO struct {
	ID  int64  `json:"stamp_id"`
	URL string `json:"stamp_url"`
}

type requiredList[T any] struct {
	Items   []T
	Present bool
	Valid   bool
}

func (l *requiredList[T]) UnmarshalJSON(data []byte) error {
	*l = requiredList[T]{Present: true}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(data, &l.Items); err != nil {
		return err
	}
	l.Valid = true
	return nil
}

func validResourceURL(rawURL string) bool {
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Hostname() == "" {
		return false
	}
	// stamps 资源必须落在既有 Pixiv media host allowlist 内，避免把不受信任
	// 的 locator 交给后续 SDK/resource 层；该 allowlist 与 sdk/pixiv 保持一致。
	switch strings.ToLower(parsed.Hostname()) {
	case "i.pximg.net", "s.pximg.net", "i-f.pximg.net":
	default:
		return false
	}
	return parsed.Path != "" && parsed.Path != "/"
}
