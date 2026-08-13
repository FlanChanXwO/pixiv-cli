package follow

import (
	"context"
	"errors"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

// Transport 是 user follow mutation 所需的最小 form 传输能力。
type Transport interface {
	PostForm(context.Context, string, url.Values) error
}

type Client struct{ transport Transport }

func New(transport Transport) *Client { return &Client{transport: transport} }

type Request struct {
	UserID   int64
	Restrict string
}

// Add 发送关注请求；成功语义由 appapi transport 的 HTTP 状态码决定。
func (c *Client) Add(ctx context.Context, request Request) error {
	if c == nil || c.transport == nil {
		return errors.New("user follow transport is not configured")
	}
	return c.transport.PostForm(ctx, protocol.AppFollowAdd, url.Values{
		"user_id":  {strconv.FormatInt(request.UserID, 10)},
		"restrict": {request.Restrict},
	})
}

// Remove 取消关注；不解释或伪造上游响应正文。
func (c *Client) Remove(ctx context.Context, userID int64) error {
	if c == nil || c.transport == nil {
		return errors.New("user follow transport is not configured")
	}
	return c.transport.PostForm(ctx, protocol.AppFollowDelete, url.Values{
		"user_id": {strconv.FormatInt(userID, 10)},
	})
}
