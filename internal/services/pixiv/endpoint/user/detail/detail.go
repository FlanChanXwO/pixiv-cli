package detail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

type Transport interface {
	GetJSON(context.Context, string, url.Values, any) error
}

type Client struct{ transport Transport }

func New(transport Transport) *Client { return &Client{transport: transport} }

func (c *Client) Detail(ctx context.Context, userID int64) (user.Detail, error) {
	return c.get(ctx, protocol.AppUserDetail, url.Values{"user_id": {strconv.FormatInt(userID, 10)}})
}

func (c *Client) Current(ctx context.Context) (user.Detail, error) {
	return c.get(ctx, protocol.AppUserMe, nil)
}

func (c *Client) get(ctx context.Context, path string, query url.Values) (user.Detail, error) {
	if c == nil || c.transport == nil {
		return user.Detail{}, errors.New("user detail transport is not configured")
	}
	var raw responseDTO
	if err := c.transport.GetJSON(ctx, path, query, &raw); err != nil {
		return user.Detail{}, err
	}
	if !raw.User.Present || !raw.User.Valid || raw.User.Value.ID <= 0 || !raw.Profile.Present || !raw.Profile.Valid || !raw.ProfilePublicity.Present || !raw.ProfilePublicity.Valid || !raw.ProfilePublicity.Value.valid() || !raw.Workspace.Present || !raw.Workspace.Valid {
		return user.Detail{}, protocol.MalformedResponse()
	}
	return user.Detail{User: mapUser(raw.User.Value), Profile: mapProfile(raw.Profile.Value), ProfilePublicity: mapProfilePublicity(raw.ProfilePublicity.Value), Workspace: mapWorkspace(raw.Workspace.Value)}, nil
}

type responseDTO struct {
	User             requiredObject[userDTO]             `json:"user"`
	Profile          requiredObject[profileDTO]          `json:"profile"`
	ProfilePublicity requiredObject[profilePublicityDTO] `json:"profile_publicity"`
	Workspace        requiredObject[workspaceDTO]        `json:"workspace"`
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
type profileDTO struct {
	Webpage                    *string `json:"webpage"`
	Gender                     string  `json:"gender"`
	Birth                      string  `json:"birth"`
	BirthDay                   string  `json:"birth_day"`
	BirthYear                  int     `json:"birth_year"`
	Region                     string  `json:"region"`
	AddressID                  int64   `json:"address_id"`
	CountryCode                string  `json:"country_code"`
	Job                        string  `json:"job"`
	JobID                      int64   `json:"job_id"`
	TotalFollowUsers           int     `json:"total_follow_users"`
	TotalMyPixivUsers          int     `json:"total_mypixiv_users"`
	TotalIllusts               int     `json:"total_illusts"`
	TotalManga                 int     `json:"total_manga"`
	TotalNovels                int     `json:"total_novels"`
	TotalIllustBookmarksPublic int     `json:"total_illust_bookmarks_public"`
	TotalIllustSeries          int     `json:"total_illust_series"`
	TotalNovelSeries           int     `json:"total_novel_series"`
	BackgroundImageURL         *string `json:"background_image_url"`
	TwitterAccount             string  `json:"twitter_account"`
	TwitterURL                 *string `json:"twitter_url"`
	PawooURL                   *string `json:"pawoo_url"`
	IsPremium                  bool    `json:"is_premium"`
	IsUsingCustomProfileImage  bool    `json:"is_using_custom_profile_image"`
}
type profilePublicityDTO struct {
	Gender    profilePublicityValue `json:"gender"`
	Region    profilePublicityValue `json:"region"`
	BirthDay  profilePublicityValue `json:"birth_day"`
	BirthYear profilePublicityValue `json:"birth_year"`
	Job       profilePublicityValue `json:"job"`
	Pawoo     profilePublicityValue `json:"pawoo"`
}
type profilePublicityValue struct {
	Value   bool
	Present bool
	Valid   bool
}

func (v *profilePublicityValue) UnmarshalJSON(data []byte) error {
	*v = profilePublicityValue{Present: true}
	switch string(bytes.TrimSpace(data)) {
	case "true":
		v.Value, v.Valid = true, true
		return nil
	case "false":
		v.Valid = true
		return nil
	}
	var visibility string
	if err := json.Unmarshal(data, &visibility); err != nil {
		return nil
	}
	switch visibility {
	case "public":
		v.Value, v.Valid = true, true
	case "private":
		v.Valid = true
	}
	return nil
}
func (d profilePublicityDTO) valid() bool {
	return (!d.Gender.Present || d.Gender.Valid) && (!d.Region.Present || d.Region.Valid) && (!d.BirthDay.Present || d.BirthDay.Valid) && (!d.BirthYear.Present || d.BirthYear.Valid) && (!d.Job.Present || d.Job.Valid) && (!d.Pawoo.Present || d.Pawoo.Valid)
}

type workspaceDTO struct {
	PC                string  `json:"pc"`
	Monitor           string  `json:"monitor"`
	Tool              string  `json:"tool"`
	Scanner           string  `json:"scanner"`
	Tablet            string  `json:"tablet"`
	Mouse             string  `json:"mouse"`
	Printer           string  `json:"printer"`
	Desktop           string  `json:"desktop"`
	Music             string  `json:"music"`
	Desk              string  `json:"desk"`
	Chair             string  `json:"chair"`
	Comment           string  `json:"comment"`
	WorkspaceImageURL *string `json:"workspace_image_url"`
}
type requiredObject[T any] struct {
	Value   T
	Present bool
	Valid   bool
}

func (o *requiredObject[T]) UnmarshalJSON(data []byte) error {
	*o = requiredObject[T]{Present: true}
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(data, &o.Value); err != nil {
		return err
	}
	o.Valid = true
	return nil
}
func mapUser(value userDTO) user.User {
	return user.User{ID: value.ID, Name: value.Name, Account: value.Account, Comment: value.Comment, IsFollowed: value.IsFollowed, ProfileImageURLs: user.ProfileImageURLs{Medium: cloneString(value.ProfileImageURLs.Medium)}}
}
func mapProfile(value profileDTO) user.Profile {
	return user.Profile{Webpage: cloneString(value.Webpage), Gender: value.Gender, Birth: value.Birth, BirthDay: value.BirthDay, BirthYear: value.BirthYear, Region: value.Region, AddressID: value.AddressID, CountryCode: value.CountryCode, Job: value.Job, JobID: value.JobID, TotalFollowUsers: value.TotalFollowUsers, TotalMyPixivUsers: value.TotalMyPixivUsers, TotalIllusts: value.TotalIllusts, TotalManga: value.TotalManga, TotalNovels: value.TotalNovels, TotalIllustBookmarksPublic: value.TotalIllustBookmarksPublic, TotalIllustSeries: value.TotalIllustSeries, TotalNovelSeries: value.TotalNovelSeries, BackgroundImageURL: cloneString(value.BackgroundImageURL), TwitterAccount: value.TwitterAccount, TwitterURL: cloneString(value.TwitterURL), PawooURL: cloneString(value.PawooURL), IsPremium: value.IsPremium, IsUsingCustomProfileImage: value.IsUsingCustomProfileImage}
}
func mapProfilePublicity(value profilePublicityDTO) user.ProfilePublicity {
	return user.ProfilePublicity{Gender: value.Gender.Value, Region: value.Region.Value, BirthDay: value.BirthDay.Value, BirthYear: value.BirthYear.Value, Job: value.Job.Value, Pawoo: value.Pawoo.Value}
}
func mapWorkspace(value workspaceDTO) user.Workspace {
	return user.Workspace{PC: value.PC, Monitor: value.Monitor, Tool: value.Tool, Scanner: value.Scanner, Tablet: value.Tablet, Mouse: value.Mouse, Printer: value.Printer, Desktop: value.Desktop, Music: value.Music, Desk: value.Desk, Chair: value.Chair, Comment: value.Comment, WorkspaceImageURL: cloneString(value.WorkspaceImageURL)}
}
func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
