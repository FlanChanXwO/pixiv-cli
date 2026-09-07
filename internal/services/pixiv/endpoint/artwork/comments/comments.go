package comments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

type Transport interface {
	GetJSON(context.Context, string, url.Values, any) error
	PostFormJSON(context.Context, string, url.Values, any) error
	PostForm(context.Context, string, url.Values) error
}

type Client struct{ transport Transport }

func New(transport Transport) *Client { return &Client{transport: transport} }

type Request struct {
	ArtworkID int64
	Offset    int
}

type Result struct {
	Items         []artwork.Comment
	NextOffset    int
	HasNext       bool
	Total         *int64
	AccessControl *artwork.CommentAccessControl
}

type CreateRequest struct {
	ArtworkID int64
	Comment   string
}

type ReplyRequest struct {
	ArtworkID       int64
	Comment         string
	ParentCommentID int64
}

type StampRequest struct {
	ArtworkID int64
	Comment   string
	StampID   int64
}

type MutationResult struct {
	CommentID int64
}

func (c *Client) List(ctx context.Context, request Request) (Result, error) {
	if c == nil || c.transport == nil {
		return Result{}, errors.New("artwork comments transport is not configured")
	}
	if err := validateArtworkID(request.ArtworkID); err != nil {
		return Result{}, err
	}
	if request.Offset < 0 {
		return Result{}, errors.New("comments offset must not be negative")
	}
	query := url.Values{"illust_id": {strconv.FormatInt(request.ArtworkID, 10)}}
	if request.Offset > 0 {
		query.Set("offset", strconv.Itoa(request.Offset))
	}
	var raw responseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppIllustComments, query, &raw); err != nil {
		return Result{}, err
	}
	if !raw.Comments.Present || !raw.Comments.Valid {
		return Result{}, protocol.MalformedResponse()
	}
	for _, item := range raw.Comments.Items {
		if !validCommentChain(item) {
			return Result{}, protocol.MalformedResponse()
		}
	}
	items := make([]artwork.Comment, len(raw.Comments.Items))
	for index, item := range raw.Comments.Items {
		items[index] = mapComment(item)
	}
	var total *int64
	if raw.TotalComments != nil {
		value := *raw.TotalComments
		total = &value
	}
	var accessControl *artwork.CommentAccessControl
	if raw.AccessControl != nil {
		accessControl = &artwork.CommentAccessControl{CanComment: raw.AccessControl.CanComment, IsLocked: raw.AccessControl.IsLocked}
	}
	nextOffset, hasNext, err := continuation(raw.NextURL)
	if err != nil {
		return Result{}, err
	}
	return Result{Items: items, NextOffset: nextOffset, HasNext: hasNext, Total: total, AccessControl: accessControl}, nil
}

func (c *Client) Create(ctx context.Context, request CreateRequest) (MutationResult, error) {
	if c == nil || c.transport == nil {
		return MutationResult{}, errors.New("artwork comments transport is not configured")
	}
	if err := validateArtworkID(request.ArtworkID); err != nil {
		return MutationResult{}, err
	}
	if err := validateCommentBody(request.Comment); err != nil {
		return MutationResult{}, err
	}
	form := url.Values{
		"illust_id": {strconv.FormatInt(request.ArtworkID, 10)},
		"comment":   {request.Comment},
	}
	return c.postComment(ctx, form)
}

func (c *Client) Reply(ctx context.Context, request ReplyRequest) (MutationResult, error) {
	if c == nil || c.transport == nil {
		return MutationResult{}, errors.New("artwork comments transport is not configured")
	}
	if err := validateArtworkID(request.ArtworkID); err != nil {
		return MutationResult{}, err
	}
	if err := validateCommentBody(request.Comment); err != nil {
		return MutationResult{}, err
	}
	if request.ParentCommentID <= 0 {
		return MutationResult{}, errors.New("parent comment ID must be positive")
	}
	return c.postComment(ctx, url.Values{
		"illust_id":         {strconv.FormatInt(request.ArtworkID, 10)},
		"comment":           {request.Comment},
		"parent_comment_id": {strconv.FormatInt(request.ParentCommentID, 10)},
	})
}

func (c *Client) Stamp(ctx context.Context, request StampRequest) (MutationResult, error) {
	if c == nil || c.transport == nil {
		return MutationResult{}, errors.New("artwork comments transport is not configured")
	}
	if err := validateArtworkID(request.ArtworkID); err != nil {
		return MutationResult{}, err
	}
	if err := validateCommentBody(request.Comment); err != nil {
		return MutationResult{}, err
	}
	if request.StampID <= 0 {
		return MutationResult{}, errors.New("stamp ID must be positive")
	}
	return c.postComment(ctx, url.Values{
		"illust_id": {strconv.FormatInt(request.ArtworkID, 10)},
		"comment":   {request.Comment},
		"stamp_id":  {strconv.FormatInt(request.StampID, 10)},
	})
}

func (c *Client) Delete(ctx context.Context, commentID int64) error {
	if c == nil || c.transport == nil {
		return errors.New("artwork comments transport is not configured")
	}
	if commentID <= 0 {
		return errors.New("comment ID must be positive")
	}
	return c.transport.PostForm(ctx, protocol.AppIllustCommentDelete, url.Values{
		"comment_id": {strconv.FormatInt(commentID, 10)},
	})
}

func (c *Client) postComment(ctx context.Context, form url.Values) (MutationResult, error) {
	var raw mutationResponseDTO
	if err := c.transport.PostFormJSON(ctx, protocol.AppIllustCommentAdd, form, &raw); err != nil {
		return MutationResult{}, err
	}
	if raw.CommentID == nil || *raw.CommentID <= 0 {
		return MutationResult{}, protocol.MalformedResponse()
	}
	return MutationResult{CommentID: *raw.CommentID}, nil
}

func validateArtworkID(id int64) error {
	if id <= 0 {
		return errors.New("comments artwork ID must be positive")
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

type commentAccessControlDTO struct {
	CanComment bool `json:"can_comment"`
	IsLocked   bool `json:"is_locked"`
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

func mapComment(value commentDTO) artwork.Comment {
	result := artwork.Comment{ID: value.ID, User: mapUser(value.User), Comment: value.Comment, CreateDate: value.CreateDate}
	if result.Comment == "" {
		result.Comment = value.Caption
	}
	if value.ParentComment != nil {
		parent := mapComment(*value.ParentComment)
		result.ParentComment = &parent
	}
	return result
}

func mapUser(value userDTO) artwork.UserSummary {
	return artwork.UserSummary{ID: value.ID, Name: value.Name, Account: value.Account, Comment: value.Comment,
		IsFollowed: value.IsFollowed, ProfileImageURLs: artwork.ProfileImageURLs{Medium: value.ProfileImageURLs.Medium}}
}

func continuation(rawURL *string) (int, bool, error) {
	if rawURL == nil {
		return 0, false, nil
	}
	if *rawURL == "" {
		return 0, false, protocol.MalformedResponse()
	}
	parsed, err := url.Parse(*rawURL)
	if err != nil {
		return 0, false, protocol.MalformedResponse()
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(values["offset"]) != 1 {
		return 0, false, protocol.MalformedResponse()
	}
	value, err := strconv.ParseInt(values.Get("offset"), 10, 64)
	if err != nil || value <= 0 || int64(int(value)) != value {
		return 0, false, protocol.MalformedResponse()
	}
	return int(value), true, nil
}
