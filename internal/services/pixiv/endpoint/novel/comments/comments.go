package comments

import (
	"context"
	"errors"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

type Transport interface {
	GetJSON(context.Context, string, url.Values, any) error
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

type Client struct{ transport Transport }

func New(transport Transport) *Client { return &Client{transport: transport} }

func (c *Client) List(ctx context.Context, request Request) (Result, error) {
	if c == nil || c.transport == nil {
		return Result{}, errors.New("novel comments transport is not configured")
	}
	query := url.Values{"novel_id": {strconv.FormatInt(request.NovelID, 10)}}
	if request.Offset > 0 {
		query.Set("offset", strconv.Itoa(request.Offset))
	}
	var raw responseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppNovelComments, query, &raw); err != nil {
		return Result{}, err
	}
	for _, value := range raw.Comments {
		if !validCommentChain(value) {
			return Result{}, protocol.MalformedResponse()
		}
	}
	items := make([]novel.Comment, len(raw.Comments))
	for index, value := range raw.Comments {
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

type responseDTO struct {
	Comments      []commentDTO             `json:"comments"`
	NextURL       *string                  `json:"next_url"`
	TotalComments *int64                   `json:"total_comments"`
	AccessControl *commentAccessControlDTO `json:"access_control"`
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
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return 0, protocol.MalformedResponse()
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(values["offset"]) != 1 {
		return 0, protocol.MalformedResponse()
	}
	offset, err := strconv.ParseInt(values.Get("offset"), 10, 64)
	if err != nil || offset <= 0 || int64(int(offset)) != offset {
		return 0, protocol.MalformedResponse()
	}
	return int(offset), nil
}
