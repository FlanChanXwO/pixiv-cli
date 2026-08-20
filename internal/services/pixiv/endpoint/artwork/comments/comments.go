package comments

import (
	"context"
	"errors"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

type Transport interface {
	GetJSON(context.Context, string, url.Values, any) error
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

func (c *Client) List(ctx context.Context, request Request) (Result, error) {
	if c == nil || c.transport == nil {
		return Result{}, errors.New("artwork comments transport is not configured")
	}
	query := url.Values{"illust_id": {strconv.FormatInt(request.ArtworkID, 10)}}
	if request.Offset > 0 {
		query.Set("offset", strconv.Itoa(request.Offset))
	}
	var raw responseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppIllustComments, query, &raw); err != nil {
		return Result{}, err
	}
	for _, item := range raw.Comments {
		if !validCommentChain(item) {
			return Result{}, protocol.MalformedResponse()
		}
	}
	items := make([]artwork.Comment, len(raw.Comments))
	for index, item := range raw.Comments {
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
