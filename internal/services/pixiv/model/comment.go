package model

// Comment 是作品或小说评论区单条评论的规范化表示。
type Comment struct {
	ID            int64    `json:"id"`
	User          User     `json:"user"`
	Comment       string   `json:"comment"`
	CreateDate    string   `json:"create_date"`
	ParentComment *Comment `json:"parent_comment,omitempty"`
}

// CommentAccessControl 记录上游评论页给出的访问控制状态；仅在上游显式提供时非 nil。
type CommentAccessControl struct {
	CanComment bool `json:"can_comment"`
	IsLocked   bool `json:"is_locked"`
}

// CommentList 是一个评论批次，附带可选的上游总数与访问控制元数据。
// Total 与 AccessControl 只在上游显式提供时非 nil，绝不伪造为零值。
type CommentList struct {
	Comments           []Comment             `json:"comments"`
	NextOffset         int                   `json:"-"`
	ContinuationExists bool                  `json:"-"`
	Total              *int64                `json:"total_comments,omitempty"`
	AccessControl      *CommentAccessControl `json:"access_control,omitempty"`
}
