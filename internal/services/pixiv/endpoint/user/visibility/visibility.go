package visibility

import (
	"context"
	"errors"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

// Transport 是 AI artwork visibility mutation 所需的最小 form 传输能力。
type Transport interface {
	PostForm(context.Context, string, url.Values) error
}

type Client struct{ transport Transport }

func New(transport Transport) *Client { return &Client{transport: transport} }

// Set 设置当前账号 feed 是否展示 AI 作品。
func (c *Client) Set(ctx context.Context, visible bool) error {
	if c == nil || c.transport == nil {
		return errors.New("user visibility transport is not configured")
	}
	value := "0"
	if visible {
		value = "1"
	}
	return c.transport.PostForm(ctx, protocol.AppEditAIShowSettings, url.Values{"ai_show": {value}})
}
