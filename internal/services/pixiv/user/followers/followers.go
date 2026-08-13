package followers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/user"
)

type Transport interface {
	GetJSON(context.Context, string, url.Values, any) error
}
type Request struct {
	UserID   int64
	Restrict string
	Offset   int
}
type Result struct {
	Items      []user.Preview
	NextOffset int
	HasNext    bool
}
type Client struct{ transport Transport }

func New(transport Transport) *Client { return &Client{transport: transport} }
func (c *Client) List(ctx context.Context, request Request) (Result, error) {
	if c == nil || c.transport == nil {
		return Result{}, errors.New("user followers transport is not configured")
	}
	query := url.Values{"user_id": {strconv.FormatInt(request.UserID, 10)}, "restrict": {request.Restrict}}
	if request.Offset > 0 {
		query.Set("offset", strconv.Itoa(request.Offset))
	}
	var raw responseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppUserFollower, query, &raw); err != nil {
		return Result{}, err
	}
	if !raw.Users.Present || !raw.Users.Valid {
		return Result{}, protocol.MalformedResponse()
	}
	items := make([]user.Preview, len(raw.Users.Items))
	for index, value := range raw.Users.Items {
		if value.User.ID <= 0 {
			return Result{}, protocol.MalformedResponse()
		}
		items[index] = user.Preview{User: mapUser(value.User)}
	}
	result := Result{Items: items}
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
	Users   requiredList[userPreviewDTO] `json:"user_previews"`
	NextURL *string                      `json:"next_url"`
}
type userPreviewDTO struct {
	User userDTO `json:"user"`
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
func mapUser(value userDTO) user.User {
	return user.User{ID: value.ID, Name: value.Name, Account: value.Account, Comment: value.Comment, IsFollowed: value.IsFollowed, ProfileImageURLs: user.ProfileImageURLs{Medium: cloneString(value.ProfileImageURLs.Medium)}}
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
	value, err := strconv.ParseInt(values.Get("offset"), 10, 64)
	if err != nil || value <= 0 || int64(int(value)) != value {
		return 0, protocol.MalformedResponse()
	}
	return int(value), nil
}
