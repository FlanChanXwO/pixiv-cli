package comments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"

	endpointcontinuation "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/continuation"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

type Transport interface {
	GetJSON(context.Context, string, url.Values, any) error
	PostFormJSON(context.Context, string, url.Values, any) error
	PostForm(context.Context, string, url.Values) error
}

type Request struct {
	NovelID int64
	Offset  int
}

type Result struct {
	Items         []novel.Comment
	NextOffset    int
	HasNext       bool
	Total         *int64
	AccessControl *novel.CommentAccessControl
}

type CreateRequest struct {
	NovelID int64
	Comment string
}

type ReplyRequest struct {
	NovelID         int64
	Comment         string
	ParentCommentID int64
}

type StampRequest struct {
	NovelID int64
	Comment string
	StampID int64
}

type MutationResult struct {
	CommentID int64
}

type Client struct{ transport Transport }

func New(transport Transport) *Client { return &Client{transport: transport} }

func (c *Client) List(ctx context.Context, request Request) (Result, error) {
	if c == nil || c.transport == nil {
		return Result{}, errors.New("novel comments transport is not configured")
	}
	if err := validateNovelID(request.NovelID); err != nil {
		return Result{}, err
	}
	if request.Offset < 0 {
		return Result{}, errors.New("comments offset must not be negative")
	}
	query := url.Values{"novel_id": {strconv.FormatInt(request.NovelID, 10)}}
	if request.Offset > 0 {
		query.Set("offset", strconv.Itoa(request.Offset))
	}
	var raw responseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppNovelComments, query, &raw); err != nil {
		return Result{}, err
	}
	if !raw.Comments.Present || !raw.Comments.Valid {
		return Result{}, protocol.MalformedResponse()
	}
	for _, value := range raw.Comments.Items {
		if !validCommentChain(value) {
			return Result{}, protocol.MalformedResponse()
		}
	}
	items := make([]novel.Comment, len(raw.Comments.Items))
	for index, value := range raw.Comments.Items {
		items[index] = mapComment(value)
	}
	result := Result{Items: items}
	if raw.TotalComments != nil {
		value := *raw.TotalComments
		result.Total = &value
	}
	if raw.AccessControl != nil {
		result.AccessControl = &novel.CommentAccessControl{CanComment: raw.AccessControl.CanComment, IsLocked: raw.AccessControl.IsLocked}
	}
	if raw.NextURL != nil {
		if *raw.NextURL == "" {
			return Result{}, protocol.MalformedResponse()
		}
		next, err := continuation(*raw.NextURL)
		if err != nil {
			return Result{}, err
		}
		result.NextOffset, result.HasNext = next, true
	}
	return result, nil
}

func (c *Client) Create(ctx context.Context, request CreateRequest) (MutationResult, error) {
	if c == nil || c.transport == nil {
		return MutationResult{}, errors.New("novel comments transport is not configured")
	}
	if err := validateNovelID(request.NovelID); err != nil {
		return MutationResult{}, err
	}
	if err := validateCommentBody(request.Comment); err != nil {
		return MutationResult{}, err
	}
	return c.postComment(ctx, url.Values{
		"novel_id": {strconv.FormatInt(request.NovelID, 10)},
		"comment":  {request.Comment},
	})
}

func (c *Client) Reply(ctx context.Context, request ReplyRequest) (MutationResult, error) {
	if c == nil || c.transport == nil {
		return MutationResult{}, errors.New("novel comments transport is not configured")
	}
	if err := validateNovelID(request.NovelID); err != nil {
		return MutationResult{}, err
	}
	if err := validateCommentBody(request.Comment); err != nil {
		return MutationResult{}, err
	}
	if request.ParentCommentID <= 0 {
		return MutationResult{}, errors.New("parent comment ID must be positive")
	}
	return c.postComment(ctx, url.Values{
		"novel_id":          {strconv.FormatInt(request.NovelID, 10)},
		"comment":           {request.Comment},
		"parent_comment_id": {strconv.FormatInt(request.ParentCommentID, 10)},
	})
}

func (c *Client) Stamp(ctx context.Context, request StampRequest) (MutationResult, error) {
	if c == nil || c.transport == nil {
		return MutationResult{}, errors.New("novel comments transport is not configured")
	}
	if err := validateNovelID(request.NovelID); err != nil {
		return MutationResult{}, err
	}
	if err := validateCommentBody(request.Comment); err != nil {
		return MutationResult{}, err
	}
	if request.StampID <= 0 {
		return MutationResult{}, errors.New("stamp ID must be positive")
	}
	return c.postComment(ctx, url.Values{
		"novel_id": {strconv.FormatInt(request.NovelID, 10)},
		"comment":  {request.Comment},
		"stamp_id": {strconv.FormatInt(request.StampID, 10)},
	})
}

func (c *Client) Delete(ctx context.Context, commentID int64) error {
	if c == nil || c.transport == nil {
		return errors.New("novel comments transport is not configured")
	}
	if commentID <= 0 {
		return errors.New("comment ID must be positive")
	}
	return c.transport.PostForm(ctx, protocol.AppNovelCommentDelete, url.Values{
		"comment_id": {strconv.FormatInt(commentID, 10)},
	})
}

func (c *Client) postComment(ctx context.Context, form url.Values) (MutationResult, error) {
	var raw mutationResponseDTO
	if err := c.transport.PostFormJSON(ctx, protocol.AppNovelCommentAdd, form, &raw); err != nil {
		return MutationResult{}, err
	}
	if raw.CommentID == nil || *raw.CommentID <= 0 {
		return MutationResult{}, protocol.MalformedResponse()
	}
	return MutationResult{CommentID: *raw.CommentID}, nil
}

func validateNovelID(id int64) error {
	if id <= 0 {
		return errors.New("comments novel ID must be positive")
	}
	return nil
}

func validateCommentBody(body string) error {
	// 空字符串代表调用方未提供必填正文；不裁剪空白，以免替未知的业务接受性
	// 增加未经证实的限制，具体由上游决定。
	if body == "" {
		return errors.New("comment body must not be empty")
	}
	return nil
}

type responseDTO struct {
	Comments      requiredList[commentDTO] `json:"comments"`
	NextURL       *string                  `json:"next_url"`
	TotalComments *int64                   `json:"total_comments"`
	AccessControl *commentAccessControlDTO `json:"access_control"`
}

type mutationResponseDTO struct {
	CommentID *int64 `json:"comment_id"`
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

type commentDTO struct {
	ID            int64       `json:"id"`
	User          userDTO     `json:"user"`
	Comment       string      `json:"comment"`
	Caption       string      `json:"caption"`
	CreateDate    string      `json:"created_at"`
	ParentComment *commentDTO `json:"parent_comment"`
}

type userDTO struct {
	ID               int64               `json:"id"`
	Name             string              `json:"name"`
	Account          string              `json:"account"`
	Comment          string              `json:"comment"`
	IsFollowed       bool                `json:"is_followed"`
	ProfileImageURLs profileImageURLsDTO `json:"profile_image_urls"`
}
type profileImageURLsDTO struct {
	Medium *string `json:"medium"`
}
type commentAccessControlDTO struct {
	CanComment bool `json:"can_comment"`
	IsLocked   bool `json:"is_locked"`
}

func validCommentChain(value commentDTO) bool {
	for {
		if value.ID <= 0 {
			return false
		}
		if value.ParentComment == nil {
			return true
		}
		value = *value.ParentComment
	}
}

func mapComment(value commentDTO) novel.Comment {
	result := novel.Comment{ID: value.ID, User: mapUser(value.User), Comment: value.Comment, CreateDate: value.CreateDate}
	if result.Comment == "" {
		result.Comment = value.Caption
	}
	if value.ParentComment != nil {
		parent := mapComment(*value.ParentComment)
		result.ParentComment = &parent
	}
	return result
}

func mapUser(value userDTO) novel.UserSummary {
	return novel.UserSummary{ID: value.ID, Name: value.Name, Account: value.Account, Comment: value.Comment, IsFollowed: value.IsFollowed, ProfileImageURLs: novel.ProfileImageURLs{Medium: cloneString(value.ProfileImageURLs.Medium)}}
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func continuation(rawURL string) (int, error) {
	_, offset, err := endpointcontinuation.Parse(rawURL, endpointcontinuation.Spec{
		Path:             protocol.AppNovelComments,
		Keys:             []string{"offset"},
		AllowedQueryKeys: []string{"novel_id"},
	})
	if err != nil || offset <= 0 || int64(int(offset)) != offset {
		return 0, protocol.MalformedResponse()
	}
	return int(offset), nil
}
