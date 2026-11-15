package record

import "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"

// CommentOutput 保留评论的 user、comment 与父评论链字段。
func CommentOutput(comment *pixiv.Comment) (Comment, error) {
	user, err := UserOutput(comment.Author)
	if err != nil {
		return Comment{}, err
	}
	out := Comment{ID: comment.ID, User: user, Comment: comment.Text, CreatedAt: comment.CreatedAt}
	if comment.Parent != nil {
		parent, err := CommentOutput(comment.Parent)
		if err != nil {
			return Comment{}, err
		}
		out.ParentComment = &parent
	}
	return out, nil
}

// StampOutput 只输出贴纸的稳定引用，保留既有 CLI 的资源身份格式。
func StampOutput(stamp pixiv.Stamp) (Stamp, error) {
	image, err := outputImage(stamp.Image, "stamp", stamp.ID, -1, "")
	return Stamp{ID: stamp.ID, Image: image}, err
}
