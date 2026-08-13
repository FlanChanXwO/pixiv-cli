package protocol_test

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/post"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/post/home"
	postinfo "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/post/info"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/protocol"
)

func testPost(session *protocol.Session, ctx context.Context, postID string) (post.Post, error) {
	return postinfo.New(session).Get(ctx, postinfo.Request{PostID: postID})
}

func testHome(session *protocol.Session, ctx context.Context, nextURL string) (post.Page, error) {
	return home.New(session).List(ctx, home.Request{NextURL: nextURL})
}
