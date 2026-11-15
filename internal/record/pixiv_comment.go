package record

import "time"

// Comment 保存评论及父评论链；缺失的 ParentComment 输出为 null。
type Comment struct {
	ID            int64     `json:"id"`
	User          User      `json:"user"`
	Comment       string    `json:"comment"`
	CreatedAt     time.Time `json:"created_at"`
	ParentComment *Comment  `json:"parent_comment"`
}
