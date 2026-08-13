package pixiv

import (
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// TagDTO is the output-safe form of Tag.
type TagDTO struct {
	Name           string `json:"name"`
	TranslatedName string `json:"translated_name"`
}

// ImageResourceDTO is the output-safe form of ImageResource.
type ImageResourceDTO struct {
	Resource *sdk.ResourceDTO `json:"resource"`
	Variant  string           `json:"variant"`
	Width    int              `json:"width"`
	Height   int              `json:"height"`
}

// ArtworkPageDTO is the output-safe form of ArtworkPage.
type ArtworkPageDTO struct {
	PageIndex int              `json:"page_index"`
	Image     ImageResourceDTO `json:"image"`
	Width     int              `json:"width"`
	Height    int              `json:"height"`
}

// ArtworkDTO is the output-safe form of Artwork. It intentionally contains
// only nested DTOs and stable scalar metadata.
type ArtworkDTO struct {
	ID             int64            `json:"id"`
	Title          string           `json:"title"`
	Caption        string           `json:"caption"`
	Kind           ArtworkKind      `json:"kind"`
	RawKind        string           `json:"raw_kind"`
	Tags           []TagDTO         `json:"tags"`
	User           UserDTO          `json:"user"`
	PublishedAt    time.Time        `json:"published_at"`
	UpdatedAt      *time.Time       `json:"updated_at"`
	TotalBookmarks int              `json:"total_bookmarks"`
	TotalViews     int              `json:"total_views"`
	Width          int              `json:"width"`
	Height         int              `json:"height"`
	PageCount      int              `json:"page_count"`
	XRestrict      int              `json:"x_restrict"`
	AIType         int              `json:"ai_type"`
	Tools          []string         `json:"tools"`
	Cover          ImageResourceDTO `json:"cover"`
	Pages          []ArtworkPageDTO `json:"pages"`
}

// NovelDTO is the output-safe form of Novel.
type NovelDTO struct {
	ID             int64            `json:"id"`
	Title          string           `json:"title"`
	Caption        string           `json:"caption"`
	User           UserDTO          `json:"user"`
	Tags           []TagDTO         `json:"tags"`
	PublishedAt    time.Time        `json:"published_at"`
	UpdatedAt      *time.Time       `json:"updated_at"`
	XRestrict      int              `json:"x_restrict"`
	TextLength     int              `json:"text_length"`
	IsOriginal     bool             `json:"is_original"`
	TotalBookmarks int              `json:"total_bookmarks"`
	TotalViews     int              `json:"total_views"`
	Cover          ImageResourceDTO `json:"cover"`
}

// UserDTO is the output-safe form of User.
type UserDTO struct {
	ID           int64            `json:"id"`
	Name         string           `json:"name"`
	Account      string           `json:"account"`
	Comment      string           `json:"comment"`
	IsFollowed   bool             `json:"is_followed"`
	ProfileImage ImageResourceDTO `json:"profile_image"`
}

// UserProfileDTO is the output-safe form of UserProfile.
type UserProfileDTO struct {
	Webpage                   string `json:"webpage"`
	Gender                    string `json:"gender"`
	BirthDay                  string `json:"birth_day"`
	BirthYear                 int    `json:"birth_year"`
	Region                    string `json:"region"`
	CountryCode               string `json:"country_code"`
	Job                       string `json:"job"`
	TotalFollowUsers          int    `json:"total_follow_users"`
	TotalMyPixivUsers         int    `json:"total_my_pixiv_users"`
	TotalIllusts              int    `json:"total_illusts"`
	TotalManga                int    `json:"total_manga"`
	TotalNovels               int    `json:"total_novels"`
	TotalIllustBookmarks      int    `json:"total_illust_bookmarks"`
	TotalIllustSeries         int    `json:"total_illust_series"`
	TotalNovelSeries          int    `json:"total_novel_series"`
	BackgroundImageURL        string `json:"background_image_url"`
	TwitterAccount            string `json:"twitter_account"`
	TwitterURL                string `json:"twitter_url"`
	PawooURL                  string `json:"pawoo_url"`
	IsPremium                 bool   `json:"is_premium"`
	IsUsingCustomProfileImage bool   `json:"is_using_custom_profile_image"`
}

// UserProfilePublicityDTO is the output-safe form of UserProfilePublicity.
type UserProfilePublicityDTO struct {
	Gender    bool `json:"gender"`
	Region    bool `json:"region"`
	BirthDay  bool `json:"birth_day"`
	BirthYear bool `json:"birth_year"`
	Job       bool `json:"job"`
	Pawoo     bool `json:"pawoo"`
}

// UserWorkspaceDTO is the output-safe form of UserWorkspace.
type UserWorkspaceDTO struct {
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

// UserDetailDTO is the output-safe form of UserDetail.
type UserDetailDTO struct {
	User             UserDTO                 `json:"user"`
	Profile          UserProfileDTO          `json:"profile"`
	ProfilePublicity UserProfilePublicityDTO `json:"profile_publicity"`
	Workspace        UserWorkspaceDTO        `json:"workspace"`
}

// UserPreviewDTO is the output-safe form of UserPreview.
type UserPreviewDTO struct {
	User    UserDTO      `json:"user"`
	Illusts []ArtworkDTO `json:"illusts"`
	Novels  []NovelDTO   `json:"novels"`
}

// CommentDTO is the output-safe form of Comment.
type CommentDTO struct {
	ID            int64       `json:"id"`
	User          UserDTO     `json:"user"`
	Comment       string      `json:"comment"`
	CreatedAt     time.Time   `json:"created_at"`
	ParentComment *CommentDTO `json:"parent_comment"`
}

// CommentAccessControlDTO is the output-safe form of CommentAccessControl.
type CommentAccessControlDTO struct {
	CanComment bool `json:"can_comment"`
	IsLocked   bool `json:"is_locked"`
}

// CommentPageDTO is the output-safe form of CommentPage.
type CommentPageDTO struct {
	Page          sdk.PageDTO[CommentDTO]  `json:"page"`
	Total         *int64                   `json:"total"`
	AccessControl *CommentAccessControlDTO `json:"access_control"`
}

// NovelSeriesDTO is the output-safe form of NovelSeries.
type NovelSeriesDTO struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Caption     string  `json:"caption"`
	User        UserDTO `json:"user"`
	IsConcluded bool    `json:"is_concluded"`
}

// NovelSeriesResultDTO is the output-safe form of NovelSeriesResult.
type NovelSeriesResultDTO struct {
	Series NovelSeriesDTO        `json:"series"`
	Novels sdk.PageDTO[NovelDTO] `json:"novels"`
}

// BookmarkTagDTO is the output-safe form of BookmarkTag.
type BookmarkTagDTO struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// TrendingTagDTO is the output-safe form of TrendingTag.
type TrendingTagDTO struct {
	Tag            string     `json:"tag"`
	TranslatedName string     `json:"translated_name"`
	Artwork        ArtworkDTO `json:"artwork"`
}

// ArtworkBookmarkDetailDTO is the output-safe form of ArtworkBookmarkDetail.
type ArtworkBookmarkDetailDTO struct {
	Restrict Restrict `json:"restrict"`
	Tags     []string `json:"tags"`
}

// NovelRubyDTO is the output-safe form of NovelRuby.
type NovelRubyDTO struct {
	Text     string `json:"text"`
	Furigana string `json:"furigana"`
}

// NovelMarkDTO is the output-safe form of NovelMark.
type NovelMarkDTO struct {
	Kind  NovelMarkKind `json:"kind"`
	Text  string        `json:"text"`
	Ruby  *NovelRubyDTO `json:"ruby"`
	Href  string        `json:"href"`
	Class string        `json:"class"`
}

// NovelImageBlockDTO is the output-safe form of NovelImageBlock.
type NovelImageBlockDTO struct {
	Resource *sdk.ResourceDTO `json:"resource"`
	Caption  string           `json:"caption"`
	Width    int              `json:"width"`
	Height   int              `json:"height"`
}

// NovelFileBlockDTO is the output-safe form of NovelFileBlock.
type NovelFileBlockDTO struct {
	Resource *sdk.ResourceDTO `json:"resource"`
	Filename string           `json:"filename"`
	Caption  string           `json:"caption"`
	Size     int64            `json:"size"`
}

// NovelUnknownBlockDTO is the output-safe form of NovelUnknownBlock.
type NovelUnknownBlockDTO struct {
	RawType string            `json:"raw_type"`
	Payload map[string]string `json:"payload"`
}

// NovelBlockDTO is the output-safe form of NovelBlock.
type NovelBlockDTO struct {
	Kind    NovelBlockKind        `json:"kind"`
	Text    string                `json:"text"`
	Marks   []NovelMarkDTO        `json:"marks"`
	Image   *NovelImageBlockDTO   `json:"image"`
	File    *NovelFileBlockDTO    `json:"file"`
	Unknown *NovelUnknownBlockDTO `json:"unknown"`
}

// NovelContentDTO is the output-safe form of NovelContent.
type NovelContentDTO struct {
	NovelID int64           `json:"novel_id"`
	Title   string          `json:"title"`
	Caption string          `json:"caption"`
	Blocks  []NovelBlockDTO `json:"blocks"`
}

// UgoiraArchiveDTO is the output-safe form of UgoiraArchive.
type UgoiraArchiveDTO struct {
	Quality  UgoiraQuality    `json:"quality"`
	Resource *sdk.ResourceDTO `json:"resource"`
}

// UgoiraFrameDTO is the output-safe form of UgoiraFrame.
type UgoiraFrameDTO struct {
	Filename          string `json:"filename"`
	DelayMilliseconds int    `json:"delay_milliseconds"`
}

// UgoiraMetadataDTO is the output-safe form of UgoiraMetadata.
type UgoiraMetadataDTO struct {
	ArtworkID int64              `json:"artwork_id"`
	Archives  []UgoiraArchiveDTO `json:"archives"`
	Frames    []UgoiraFrameDTO   `json:"frames"`
}

// ToTagDTO converts a Tag to an output-safe DTO.
func ToTagDTO(value Tag) TagDTO {
	return TagDTO{Name: value.Name, TranslatedName: value.TranslatedName}
}

// ToImageResourceDTO converts an image resource without exposing its runtime
// locator or request headers.
func ToImageResourceDTO(value ImageResource) ImageResourceDTO {
	return ImageResourceDTO{
		Resource: sdk.ToResourceDTO(value.Resource),
		Variant:  value.Variant,
		Width:    value.Width,
		Height:   value.Height,
	}
}

// ToArtworkPageDTO converts an artwork page to an output-safe DTO.
func ToArtworkPageDTO(value ArtworkPage) ArtworkPageDTO {
	return ArtworkPageDTO{
		PageIndex: value.PageIndex,
		Image:     ToImageResourceDTO(value.Image),
		Width:     value.Width,
		Height:    value.Height,
	}
}

// ToArtworkDTO converts an artwork field by field to an output-safe DTO.
func ToArtworkDTO(value Artwork) ArtworkDTO {
	tags := make([]TagDTO, 0, len(value.Tags))
	for _, tag := range value.Tags {
		tags = append(tags, ToTagDTO(tag))
	}
	pages := make([]ArtworkPageDTO, 0, len(value.Pages))
	for _, page := range value.Pages {
		pages = append(pages, ToArtworkPageDTO(page))
	}
	return ArtworkDTO{
		ID:             value.ID,
		Title:          value.Title,
		Caption:        value.Caption,
		Kind:           value.Kind,
		RawKind:        value.RawKind,
		Tags:           tags,
		User:           ToUserDTO(value.User),
		PublishedAt:    value.PublishedAt,
		UpdatedAt:      copyTime(value.UpdatedAt),
		TotalBookmarks: value.TotalBookmarks,
		TotalViews:     value.TotalViews,
		Width:          value.Width,
		Height:         value.Height,
		PageCount:      value.PageCount,
		XRestrict:      value.XRestrict,
		AIType:         value.AIType,
		Tools:          append([]string(nil), value.Tools...),
		Cover:          ToImageResourceDTO(value.Cover),
		Pages:          pages,
	}
}

// ToNovelDTO converts a novel field by field to an output-safe DTO.
func ToNovelDTO(value Novel) NovelDTO {
	tags := make([]TagDTO, 0, len(value.Tags))
	for _, tag := range value.Tags {
		tags = append(tags, ToTagDTO(tag))
	}
	return NovelDTO{
		ID:             value.ID,
		Title:          value.Title,
		Caption:        value.Caption,
		User:           ToUserDTO(value.User),
		Tags:           tags,
		PublishedAt:    value.PublishedAt,
		UpdatedAt:      copyTime(value.UpdatedAt),
		XRestrict:      value.XRestrict,
		TextLength:     value.TextLength,
		IsOriginal:     value.IsOriginal,
		TotalBookmarks: value.TotalBookmarks,
		TotalViews:     value.TotalViews,
		Cover:          ToImageResourceDTO(value.Cover),
	}
}

// ToUserDTO converts a user field by field to an output-safe DTO.
func ToUserDTO(value User) UserDTO {
	return UserDTO{
		ID:           value.ID,
		Name:         value.Name,
		Account:      value.Account,
		Comment:      value.Comment,
		IsFollowed:   value.IsFollowed,
		ProfileImage: ToImageResourceDTO(value.ProfileImage),
	}
}

// ToUserProfileDTO converts a user profile to an output-safe DTO.
func ToUserProfileDTO(value UserProfile) UserProfileDTO {
	return UserProfileDTO{
		Webpage:                   value.Webpage,
		Gender:                    value.Gender,
		BirthDay:                  value.BirthDay,
		BirthYear:                 value.BirthYear,
		Region:                    value.Region,
		CountryCode:               value.CountryCode,
		Job:                       value.Job,
		TotalFollowUsers:          value.TotalFollowUsers,
		TotalMyPixivUsers:         value.TotalMyPixivUsers,
		TotalIllusts:              value.TotalIllusts,
		TotalManga:                value.TotalManga,
		TotalNovels:               value.TotalNovels,
		TotalIllustBookmarks:      value.TotalIllustBookmarks,
		TotalIllustSeries:         value.TotalIllustSeries,
		TotalNovelSeries:          value.TotalNovelSeries,
		BackgroundImageURL:        value.BackgroundImageURL,
		TwitterAccount:            value.TwitterAccount,
		TwitterURL:                value.TwitterURL,
		PawooURL:                  value.PawooURL,
		IsPremium:                 value.IsPremium,
		IsUsingCustomProfileImage: value.IsUsingCustomProfileImage,
	}
}

// ToUserProfilePublicityDTO converts profile publicity flags to a DTO.
func ToUserProfilePublicityDTO(value UserProfilePublicity) UserProfilePublicityDTO {
	return UserProfilePublicityDTO{
		Gender:    value.Gender,
		Region:    value.Region,
		BirthDay:  value.BirthDay,
		BirthYear: value.BirthYear,
		Job:       value.Job,
		Pawoo:     value.Pawoo,
	}
}

// ToUserWorkspaceDTO converts workspace metadata to a DTO.
func ToUserWorkspaceDTO(value UserWorkspace) UserWorkspaceDTO {
	return UserWorkspaceDTO{
		PC:                value.PC,
		Monitor:           value.Monitor,
		Tool:              value.Tool,
		Scanner:           value.Scanner,
		Tablet:            value.Tablet,
		Mouse:             value.Mouse,
		Printer:           value.Printer,
		Desktop:           value.Desktop,
		Music:             value.Music,
		Desk:              value.Desk,
		Chair:             value.Chair,
		Comment:           value.Comment,
		WorkspaceImageURL: value.WorkspaceImageURL,
	}
}

// ToUserDetailDTO converts a user detail envelope to a DTO.
func ToUserDetailDTO(value UserDetail) UserDetailDTO {
	return UserDetailDTO{
		User:             ToUserDTO(value.User),
		Profile:          ToUserProfileDTO(value.Profile),
		ProfilePublicity: ToUserProfilePublicityDTO(value.ProfilePublicity),
		Workspace:        ToUserWorkspaceDTO(value.Workspace),
	}
}

// ToUserPreviewDTO converts a user preview envelope to a DTO.
func ToUserPreviewDTO(value UserPreview) UserPreviewDTO {
	illusts := make([]ArtworkDTO, 0, len(value.Illusts))
	for _, artwork := range value.Illusts {
		illusts = append(illusts, ToArtworkDTO(artwork))
	}
	novels := make([]NovelDTO, 0, len(value.Novels))
	for _, novel := range value.Novels {
		novels = append(novels, ToNovelDTO(novel))
	}
	return UserPreviewDTO{User: ToUserDTO(value.User), Illusts: illusts, Novels: novels}
}

// ToCommentDTO converts a comment, including its optional parent chain.
func ToCommentDTO(value Comment) CommentDTO {
	return CommentDTO{
		ID:            value.ID,
		User:          ToUserDTO(value.User),
		Comment:       value.Comment,
		CreatedAt:     value.CreatedAt,
		ParentComment: toCommentDTO(value.ParentComment),
	}
}

func toCommentDTO(value *Comment) *CommentDTO {
	if value == nil {
		return nil
	}
	dto := ToCommentDTO(*value)
	return &dto
}

// ToCommentAccessControlDTO converts comment access metadata to a DTO.
func ToCommentAccessControlDTO(value CommentAccessControl) CommentAccessControlDTO {
	return CommentAccessControlDTO{CanComment: value.CanComment, IsLocked: value.IsLocked}
}

// ToCommentPageDTO converts a comment page while preserving the stable cursor
// text and optional upstream metadata.
func ToCommentPageDTO(value CommentPage) CommentPageDTO {
	items := make([]CommentDTO, 0, len(value.Page.Items))
	for _, item := range value.Page.Items {
		items = append(items, ToCommentDTO(item))
	}
	var accessControl *CommentAccessControlDTO
	if value.AccessControl != nil {
		dto := ToCommentAccessControlDTO(*value.AccessControl)
		accessControl = &dto
	}
	return CommentPageDTO{
		Page:          sdk.PageDTO[CommentDTO]{Items: items, Next: value.Page.Next.String()},
		Total:         value.Total,
		AccessControl: accessControl,
	}
}

// ToNovelSeriesDTO converts a novel series to a DTO.
func ToNovelSeriesDTO(value NovelSeries) NovelSeriesDTO {
	return NovelSeriesDTO{
		ID:          value.ID,
		Title:       value.Title,
		Caption:     value.Caption,
		User:        ToUserDTO(value.User),
		IsConcluded: value.IsConcluded,
	}
}

// ToNovelSeriesResultDTO converts a novel series result to a DTO.
func ToNovelSeriesResultDTO(value NovelSeriesResult) NovelSeriesResultDTO {
	novels := make([]NovelDTO, 0, len(value.Novels.Items))
	for _, novel := range value.Novels.Items {
		novels = append(novels, ToNovelDTO(novel))
	}
	return NovelSeriesResultDTO{
		Series: ToNovelSeriesDTO(value.Series),
		Novels: sdk.PageDTO[NovelDTO]{Items: novels, Next: value.Novels.Next.String()},
	}
}

// ToBookmarkTagDTO converts a bookmark tag to a DTO.
func ToBookmarkTagDTO(value BookmarkTag) BookmarkTagDTO {
	return BookmarkTagDTO{Name: value.Name, Count: value.Count}
}

// ToTrendingTagDTO converts a trending tag and its sample artwork to a DTO.
func ToTrendingTagDTO(value TrendingTag) TrendingTagDTO {
	return TrendingTagDTO{Tag: value.Tag, TranslatedName: value.TranslatedName, Artwork: ToArtworkDTO(value.Artwork)}
}

// ToArtworkBookmarkDetailDTO converts bookmark state to a DTO.
func ToArtworkBookmarkDetailDTO(value ArtworkBookmarkDetail) ArtworkBookmarkDetailDTO {
	return ArtworkBookmarkDetailDTO{Restrict: value.Restrict, Tags: append([]string(nil), value.Tags...)}
}

// ToNovelRubyDTO converts a ruby annotation to a DTO.
func ToNovelRubyDTO(value NovelRuby) NovelRubyDTO {
	return NovelRubyDTO{Text: value.Text, Furigana: value.Furigana}
}

// ToNovelMarkDTO converts an inline novel mark to a DTO.
func ToNovelMarkDTO(value NovelMark) NovelMarkDTO {
	var ruby *NovelRubyDTO
	if value.Ruby != nil {
		dto := ToNovelRubyDTO(*value.Ruby)
		ruby = &dto
	}
	return NovelMarkDTO{Kind: value.Kind, Text: value.Text, Ruby: ruby, Href: value.Href, Class: value.Class}
}

// ToNovelImageBlockDTO converts an image block to a DTO.
func ToNovelImageBlockDTO(value NovelImageBlock) NovelImageBlockDTO {
	return NovelImageBlockDTO{Resource: sdk.ToResourceDTO(value.Resource), Caption: value.Caption, Width: value.Width, Height: value.Height}
}

// ToNovelFileBlockDTO converts a file block to a DTO.
func ToNovelFileBlockDTO(value NovelFileBlock) NovelFileBlockDTO {
	return NovelFileBlockDTO{Resource: sdk.ToResourceDTO(value.Resource), Filename: value.Filename, Caption: value.Caption, Size: value.Size}
}

// ToNovelUnknownBlockDTO converts an unknown block while copying its payload.
func ToNovelUnknownBlockDTO(value NovelUnknownBlock) NovelUnknownBlockDTO {
	return NovelUnknownBlockDTO{RawType: value.RawType, Payload: cloneStringMap(value.Payload)}
}

// ToNovelBlockDTO converts a novel block and all of its optional variants.
func ToNovelBlockDTO(value NovelBlock) NovelBlockDTO {
	marks := make([]NovelMarkDTO, 0, len(value.Marks))
	for _, mark := range value.Marks {
		marks = append(marks, ToNovelMarkDTO(mark))
	}
	var image *NovelImageBlockDTO
	if value.Image != nil {
		dto := ToNovelImageBlockDTO(*value.Image)
		image = &dto
	}
	var file *NovelFileBlockDTO
	if value.File != nil {
		dto := ToNovelFileBlockDTO(*value.File)
		file = &dto
	}
	var unknown *NovelUnknownBlockDTO
	if value.Unknown != nil {
		dto := ToNovelUnknownBlockDTO(*value.Unknown)
		unknown = &dto
	}
	return NovelBlockDTO{Kind: value.Kind, Text: value.Text, Marks: marks, Image: image, File: file, Unknown: unknown}
}

// ToNovelContentDTO converts structured novel content without exposing runtime
// resource metadata or retaining mutable source slices and maps.
func ToNovelContentDTO(value NovelContent) NovelContentDTO {
	blocks := make([]NovelBlockDTO, 0, len(value.Blocks))
	for _, block := range value.Blocks {
		blocks = append(blocks, ToNovelBlockDTO(block))
	}
	return NovelContentDTO{NovelID: value.NovelID, Title: value.Title, Caption: value.Caption, Blocks: blocks}
}

// ToUgoiraArchiveDTO converts an animation archive to a DTO.
func ToUgoiraArchiveDTO(value UgoiraArchive) UgoiraArchiveDTO {
	return UgoiraArchiveDTO{Quality: value.Quality, Resource: sdk.ToResourceDTO(value.Resource)}
}

// ToUgoiraFrameDTO converts an animation frame to a DTO.
func ToUgoiraFrameDTO(value UgoiraFrame) UgoiraFrameDTO {
	return UgoiraFrameDTO{Filename: value.Filename, DelayMilliseconds: value.DelayMilliseconds}
}

// ToUgoiraMetadataDTO converts animation metadata to a DTO.
func ToUgoiraMetadataDTO(value UgoiraMetadata) UgoiraMetadataDTO {
	archives := make([]UgoiraArchiveDTO, 0, len(value.Archives))
	for _, archive := range value.Archives {
		archives = append(archives, ToUgoiraArchiveDTO(archive))
	}
	frames := make([]UgoiraFrameDTO, 0, len(value.Frames))
	for _, frame := range value.Frames {
		frames = append(frames, ToUgoiraFrameDTO(frame))
	}
	return UgoiraMetadataDTO{ArtworkID: value.ArtworkID, Archives: archives, Frames: frames}
}

func copyTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func cloneStringMap(value map[string]string) map[string]string {
	out := make(map[string]string, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}
