package record

import (
	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// CommentOutput 保留评论的 user、comment 与父评论链字段。
func CommentOutput(comment *pixiv.Comment) (legacysdk.Comment, error) {
	user, err := UserOutput(comment.Author)
	if err != nil {
		return legacysdk.Comment{}, err
	}
	out := legacysdk.Comment{ID: comment.ID, User: user, Comment: comment.Text, CreatedAt: comment.CreatedAt}
	if comment.Parent != nil {
		parent, err := CommentOutput(comment.Parent)
		if err != nil {
			return legacysdk.Comment{}, err
		}
		out.ParentComment = &parent
	}
	return out, nil
}

// StampOutput 只输出贴纸的稳定引用，保留既有 CLI 的资源身份格式。
func StampOutput(stamp pixiv.Stamp) (legacysdk.Stamp, error) {
	image, err := outputImage(stamp.Image, "stamp", stamp.ID, -1, "")
	return legacysdk.Stamp{ID: stamp.ID, Image: image}, err
}
