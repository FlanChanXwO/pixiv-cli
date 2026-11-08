package pixiv

import (
	"bytes"
	"encoding/json"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
)

type userDetailWire struct {
	User             api.RequiredObject[userWire]             `json:"user"`
	Profile          api.RequiredObject[profileWire]          `json:"profile"`
	ProfilePublicity api.RequiredObject[profilePublicityWire] `json:"profile_publicity"`
	Workspace        api.RequiredObject[workspaceWire]        `json:"workspace"`
}

type profileWire struct {
	Webpage                    *string `json:"webpage"`
	Gender                     string  `json:"gender"`
	BirthDay                   string  `json:"birth_day"`
	BirthYear                  int     `json:"birth_year"`
	Region                     string  `json:"region"`
	CountryCode                string  `json:"country_code"`
	Job                        string  `json:"job"`
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

type profilePublicityWire struct {
	Gender    profileVisibility `json:"gender"`
	Region    profileVisibility `json:"region"`
	BirthDay  profileVisibility `json:"birth_day"`
	BirthYear profileVisibility `json:"birth_year"`
	Job       profileVisibility `json:"job"`
	Pawoo     profileVisibility `json:"pawoo"`
}

// profileVisibility decodes a boolean or "public"/"private" and records both
// absence and unrecognized values. A present but invalid value fails the whole
// profile response instead of defaulting to private.
type profileVisibility struct {
	Value   bool
	Present bool
	Valid   bool
}

// UnmarshalJSON records whether a supplied visibility value matches the upstream boolean or string forms.
func (v *profileVisibility) UnmarshalJSON(data []byte) error {
	*v = profileVisibility{Present: true}
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

func (d profilePublicityWire) valid() bool {
	for _, value := range []profileVisibility{d.Gender, d.Region, d.BirthDay, d.BirthYear, d.Job, d.Pawoo} {
		if value.Present && !value.Valid {
			return false
		}
	}
	return true
}

type workspaceWire struct {
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

// Profile carries a user's public profile counters and links.
type Profile struct {
	User                      *User            `json:"user"`
	Publicity                 ProfilePublicity `json:"publicity"`
	Workspace                 Workspace        `json:"workspace"`
	Webpage                   string           `json:"webpage"`
	Gender                    string           `json:"gender"`
	BirthDay                  string           `json:"birth_day"`
	BirthYear                 int              `json:"birth_year"`
	Region                    string           `json:"region"`
	CountryCode               string           `json:"country_code"`
	Job                       string           `json:"job"`
	TotalFollowUsers          int              `json:"total_follow_users"`
	TotalMyPixivUsers         int              `json:"total_my_pixiv_users"`
	TotalIllusts              int              `json:"total_illusts"`
	TotalManga                int              `json:"total_manga"`
	TotalNovels               int              `json:"total_novels"`
	TotalIllustBookmarks      int              `json:"total_illust_bookmarks"`
	TotalIllustSeries         int              `json:"total_illust_series"`
	TotalNovelSeries          int              `json:"total_novel_series"`
	BackgroundImageURL        string           `json:"background_image_url"`
	TwitterAccount            string           `json:"twitter_account"`
	TwitterURL                string           `json:"twitter_url"`
	PawooURL                  string           `json:"pawoo_url"`
	IsPremium                 bool             `json:"is_premium"`
	IsUsingCustomProfileImage bool             `json:"is_using_custom_profile_image"`
}

// ProfilePublicity records which profile fields are publicly visible.
type ProfilePublicity struct {
	Gender    bool `json:"gender"`
	Region    bool `json:"region"`
	BirthDay  bool `json:"birth_day"`
	BirthYear bool `json:"birth_year"`
	Job       bool `json:"job"`
	Pawoo     bool `json:"pawoo"`
}

// Workspace describes a user's declared workspace and tools.
type Workspace struct {
	PC                string `json:"pc"`
	Monitor           string `json:"monitor"`
	Tool              string `json:"tool"`
	Scanner           string `json:"scanner"`
	Tablet            string `json:"tablet"`
	Mouse             string `json:"mouse"`
	Printer           string `json:"printer"`
	Desktop           string `json:"desktop"`
	Music             string `json:"music"`
	Desk              string `json:"desk"`
	Chair             string `json:"chair"`
	Comment           string `json:"comment"`
	WorkspaceImageURL string `json:"workspace_image_url"`
}

func (c *Client) mapProfile(value userDetailWire) (*Profile, error) {
	user, err := c.mapUser(value.User.Value)
	if err != nil {
		return nil, err
	}
	profile := value.Profile.Value
	publicity := value.ProfilePublicity.Value
	workspace := value.Workspace.Value
	return &Profile{
		User:                      user,
		Webpage:                   strValue(profile.Webpage),
		Gender:                    profile.Gender,
		BirthDay:                  profile.BirthDay,
		BirthYear:                 profile.BirthYear,
		Region:                    profile.Region,
		CountryCode:               profile.CountryCode,
		Job:                       profile.Job,
		TotalFollowUsers:          profile.TotalFollowUsers,
		TotalMyPixivUsers:         profile.TotalMyPixivUsers,
		TotalIllusts:              profile.TotalIllusts,
		TotalManga:                profile.TotalManga,
		TotalNovels:               profile.TotalNovels,
		TotalIllustBookmarks:      profile.TotalIllustBookmarksPublic,
		TotalIllustSeries:         profile.TotalIllustSeries,
		TotalNovelSeries:          profile.TotalNovelSeries,
		BackgroundImageURL:        strValue(profile.BackgroundImageURL),
		TwitterAccount:            profile.TwitterAccount,
		TwitterURL:                strValue(profile.TwitterURL),
		PawooURL:                  strValue(profile.PawooURL),
		IsPremium:                 profile.IsPremium,
		IsUsingCustomProfileImage: profile.IsUsingCustomProfileImage,
		Publicity: ProfilePublicity{
			Gender:    publicity.Gender.Value,
			Region:    publicity.Region.Value,
			BirthDay:  publicity.BirthDay.Value,
			BirthYear: publicity.BirthYear.Value,
			Job:       publicity.Job.Value,
			Pawoo:     publicity.Pawoo.Value,
		},
		Workspace: Workspace{
			PC:                workspace.PC,
			Monitor:           workspace.Monitor,
			Tool:              workspace.Tool,
			Scanner:           workspace.Scanner,
			Tablet:            workspace.Tablet,
			Mouse:             workspace.Mouse,
			Printer:           workspace.Printer,
			Desktop:           workspace.Desktop,
			Music:             workspace.Music,
			Desk:              workspace.Desk,
			Chair:             workspace.Chair,
			Comment:           workspace.Comment,
			WorkspaceImageURL: strValue(workspace.WorkspaceImageURL),
		},
	}, nil
}

func strValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
