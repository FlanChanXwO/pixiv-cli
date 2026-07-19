package appapi

import (
	"bytes"
	"encoding/json"
)

// 以下 DTO 只表达 App API wire shape；normalized model 的所有权仍在 internal/pixiv/model。
type illustListDTO struct {
	Illusts requiredList[illustDTO] `json:"illusts"`
	NextURL *string                 `json:"next_url"`
}
type illustDetailDTO struct {
	Illust *illustDTO `json:"illust"`
}
type searchIllustOptionsDTO struct {
	Illust *searchOptionsScopeDTO `json:"illust"`
}
type searchOptionsScopeDTO struct {
	Tool *searchToolOptionsDTO `json:"tool"`
}
type searchToolOptionsDTO struct {
	Options []string `json:"options"`
}
type novelListDTO struct {
	Novels  requiredList[novelDTO] `json:"novels"`
	NextURL *string                `json:"next_url"`
}
type novelDTO struct {
	ID             int64        `json:"id"`
	Title          string       `json:"title"`
	Caption        string       `json:"caption"`
	User           userDTO      `json:"user"`
	Tags           []tagDTO     `json:"tags"`
	ImageURLs      imageURLsDTO `json:"image_urls"`
	CreateDate     string       `json:"create_date"`
	TotalBookmarks int          `json:"total_bookmarks"`
	TotalView      int          `json:"total_view"`
}
type illustDTO struct {
	ID             int64         `json:"id"`
	Title          string        `json:"title"`
	Type           string        `json:"type"`
	PageCount      int           `json:"page_count"`
	TotalBookmarks int           `json:"total_bookmarks"`
	TotalView      int           `json:"total_view"`
	XRestrict      int           `json:"x_restrict"`
	User           userDTO       `json:"user"`
	Tags           []tagDTO      `json:"tags"`
	ImageURLs      imageURLsDTO  `json:"image_urls"`
	MetaSinglePage singlePageDTO `json:"meta_single_page"`
	MetaPages      []metaPageDTO `json:"meta_pages"`
	AIType         int           `json:"ai_type"`
	CreateDate     string        `json:"create_date"`
	Width          int           `json:"width"`
	Height         int           `json:"height"`
	Tools          []string      `json:"tools"`
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
type tagDTO struct {
	Name           string `json:"name"`
	TranslatedName string `json:"translated_name"`
}
type imageURLsDTO struct {
	SquareMedium string `json:"square_medium"`
	Medium       string `json:"medium"`
	Large        string `json:"large"`
	Original     string `json:"original"`
}
type singlePageDTO struct {
	OriginalImageURL string `json:"original_image_url"`
}
type metaPageDTO struct {
	PageIndex int          `json:"page_index"`
	Width     int          `json:"width"`
	Height    int          `json:"height"`
	Extension string       `json:"extension"`
	ImageURLs imageURLsDTO `json:"image_urls"`
}
type userPreviewListDTO struct {
	UserPreviews requiredList[userPreviewDTO] `json:"user_previews"`
	NextURL      *string                      `json:"next_url"`
}
type userPreviewDTO struct {
	User userDTO `json:"user"`
}
type recommendedUserListDTO struct {
	UserPreviews requiredList[recommendedUserPreviewDTO] `json:"user_previews"`
	NextURL      *string                                 `json:"next_url"`
}
type recommendedUserPreviewDTO struct {
	User    userDTO     `json:"user"`
	Illusts []illustDTO `json:"illusts"`
	Novels  []novelDTO  `json:"novels"`
}
type trendTagsDTO struct {
	TrendTags requiredList[trendTagDTO] `json:"trend_tags"`
}
type trendTagDTO struct {
	Tag            string                    `json:"tag"`
	TranslatedName string                    `json:"translated_name"`
	Illust         requiredObject[illustDTO] `json:"illust"`
}
type ugoiraMetadataResultDTO struct {
	UgoiraMetadata requiredObject[ugoiraMetadataDTO] `json:"ugoira_metadata"`
}
type ugoiraMetadataDTO struct {
	ZipURLs requiredObject[ugoiraZipURLsDTO] `json:"zip_urls"`
	Frames  requiredList[ugoiraFrameDTO]     `json:"frames"`
}
type ugoiraZipURLsDTO struct {
	Medium string `json:"medium"`
}
type ugoiraFrameDTO struct {
	File  string `json:"file"`
	Delay int    `json:"delay"`
}
type userDetailDTO struct {
	User             requiredObject[userDTO]             `json:"user"`
	Profile          requiredObject[profileDTO]          `json:"profile"`
	ProfilePublicity requiredObject[profilePublicityDTO] `json:"profile_publicity"`
	Workspace        requiredObject[workspaceDTO]        `json:"workspace"`
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

// profilePublicityValue 隔离 App API 的 wire 差异：该接口既会返回 bool，
// 也会以 public/private 字符串表示档案可见性。公开 SDK 始终只暴露 bool。
type profilePublicityValue struct {
	Value   bool
	Present bool
	Valid   bool
}

func (v *profilePublicityValue) UnmarshalJSON(data []byte) error {
	*v = profilePublicityValue{Present: true}
	switch string(bytes.TrimSpace(data)) {
	case "true":
		v.Value = true
		v.Valid = true
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
		v.Value = true
		v.Valid = true
	case "private":
		v.Valid = true
	}
	return nil
}

func (d profilePublicityDTO) valid() bool {
	// 公开模型以零值表示上游没有给出的可见性字段；只有实际出现的字段
	// 才需要通过 wire 值校验。外层 profile_publicity object 仍由调用方强制要求。
	return (!d.Gender.Present || d.Gender.Valid) &&
		(!d.Region.Present || d.Region.Valid) &&
		(!d.BirthDay.Present || d.BirthDay.Valid) &&
		(!d.BirthYear.Present || d.BirthYear.Valid) &&
		(!d.Job.Present || d.Job.Valid) &&
		(!d.Pawoo.Present || d.Pawoo.Valid)
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

// requiredList 区分 wire 的显式空数组与缺失/null；只有前者是合法空批次。
type requiredList[T any] struct {
	Items   []T
	Present bool
	Valid   bool
}

func (l *requiredList[T]) UnmarshalJSON(data []byte) error {
	*l = requiredList[T]{}
	l.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(data, &l.Items); err != nil {
		return err
	}
	l.Valid = true
	return nil
}

// requiredObject 每次反序列化都先清空 Value，确保重复 object key 遵循 JSON 最后值而非字段合并。
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
	if len(data) == 0 || data[0] != '{' {
		return json.Unmarshal(data, &o.Value)
	}
	if err := json.Unmarshal(data, &o.Value); err != nil {
		return err
	}
	o.Valid = true
	return nil
}
