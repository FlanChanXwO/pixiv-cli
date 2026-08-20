package fanbox

import (
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// ImageResourceDTO is the output-safe form of ImageResource.
type ImageResourceDTO struct {
	Resource *sdk.ResourceDTO `json:"resource"`
	Variant  string           `json:"variant"`
	Width    int              `json:"width"`
	Height   int              `json:"height"`
}

// FileResourceDTO is the output-safe form of FileResource.
type FileResourceDTO struct {
	Resource *sdk.ResourceDTO `json:"resource"`
	Name     string           `json:"name"`
}

// CreatorSummaryDTO is the output-safe form of CreatorSummary.
type CreatorSummaryDTO struct {
	ID   string           `json:"id"`
	Name string           `json:"name"`
	Icon ImageResourceDTO `json:"icon"`
}

// CreatorDTO is the output-safe form of Creator. Embedded runtime fields are
// flattened explicitly so the DTO does not embed a runtime model.
type CreatorDTO struct {
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	Icon              ImageResourceDTO `json:"icon"`
	HasAdultContent   bool             `json:"has_adult_content"`
	IsFollowing       bool             `json:"is_following"`
	Cover             ImageResourceDTO `json:"cover"`
	PlanFee           int              `json:"plan_fee"`
	HasSupportingPlan bool             `json:"has_supporting_plan"`
}

// PostImageBlockDTO is the output-safe form of PostImageBlock.
type PostImageBlockDTO struct {
	Resource *sdk.ResourceDTO `json:"resource"`
	Caption  string           `json:"caption"`
}

// PostFileBlockDTO is the output-safe form of PostFileBlock.
type PostFileBlockDTO struct {
	Resource *sdk.ResourceDTO `json:"resource"`
	Name     string           `json:"name"`
	Caption  string           `json:"caption"`
}

// PostArticleBlockDTO is the output-safe form of PostArticleBlock.
type PostArticleBlockDTO struct {
	Text string `json:"text"`
}

// PostVideoEmbedDTO is the output-safe form of PostVideoEmbed.
type PostVideoEmbedDTO struct {
	Provider     string            `json:"provider"`
	ContentID    string            `json:"content_id"`
	CanonicalURL string            `json:"canonical_url"`
	Title        string            `json:"title"`
	ThumbnailURL string            `json:"thumbnail_url"`
	VideoID      string            `json:"video_id"`
	EmbeddedData map[string]string `json:"embedded_data"`
}

// PostUnknownBlockDTO is the output-safe form of PostUnknownBlock.
type PostUnknownBlockDTO struct {
	RawType string            `json:"raw_type"`
	Payload map[string]string `json:"payload"`
}

// PostBlockDTO is the output-safe form of PostBlock.
type PostBlockDTO struct {
	Kind    PostBlockKind        `json:"kind"`
	Image   *PostImageBlockDTO   `json:"image"`
	File    *PostFileBlockDTO    `json:"file"`
	Article *PostArticleBlockDTO `json:"article"`
	Video   *PostVideoEmbedDTO   `json:"video"`
	Unknown *PostUnknownBlockDTO `json:"unknown"`
}

// AssetDTO is the output-safe form of Asset.
type AssetDTO struct {
	ID        string           `json:"id"`
	Kind      AssetKind        `json:"kind"`
	Name      string           `json:"name"`
	Resource  *sdk.ResourceDTO `json:"resource"`
	Thumbnail ImageResourceDTO `json:"thumbnail"`
}

// PostBodyDTO is the output-safe form of PostBody.
type PostBodyDTO struct {
	Text   string         `json:"text"`
	Blocks []PostBlockDTO `json:"blocks"`
	Assets []AssetDTO     `json:"assets"`
}

// PostDTO is the output-safe form of Post.
type PostDTO struct {
	ID            string           `json:"id"`
	Title         string           `json:"title"`
	PublishedAt   time.Time        `json:"published_at"`
	CreatorID     string           `json:"creator_id"`
	FeeRequired   int              `json:"fee_required"`
	IsRestricted  bool             `json:"is_restricted"`
	IsPinned      bool             `json:"is_pinned"`
	RestrictedFor int              `json:"restricted_for"`
	CommentCount  int              `json:"comment_count"`
	Cover         ImageResourceDTO `json:"cover"`
	Body          *PostBodyDTO     `json:"body"`
}

// UserDTO is the output-safe form of User.
type UserDTO struct {
	UserID        int64  `json:"user_id"`
	DisplayName   string `json:"display_name"`
	CreatorID     string `json:"creator_id"`
	CreatorStatus string `json:"creator_status"`
	IsCreator     bool   `json:"is_creator"`
}

// CreatorTagDTO is the output-safe form of CreatorTag.
type CreatorTagDTO struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// ToImageResourceDTO converts a FANBOX image resource without exposing its
// runtime locator or request headers.
func ToImageResourceDTO(value ImageResource) ImageResourceDTO {
	return ImageResourceDTO{
		Resource: sdk.ToResourceDTO(value.Resource),
		Variant:  value.Variant,
		Width:    value.Width,
		Height:   value.Height,
	}
}

// ToFileResourceDTO converts a FANBOX file resource to a DTO.
func ToFileResourceDTO(value FileResource) FileResourceDTO {
	return FileResourceDTO{Resource: sdk.ToResourceDTO(value.Resource), Name: value.Name}
}

// ToCreatorSummaryDTO converts a creator summary to a DTO.
func ToCreatorSummaryDTO(value CreatorSummary) CreatorSummaryDTO {
	return CreatorSummaryDTO{ID: value.ID, Name: value.Name, Icon: ToImageResourceDTO(value.Icon)}
}

// ToCreatorDTO converts a creator field by field to an output-safe DTO.
func ToCreatorDTO(value Creator) CreatorDTO {
	return CreatorDTO{
		ID:                value.ID,
		Name:              value.Name,
		Icon:              ToImageResourceDTO(value.Icon),
		HasAdultContent:   value.HasAdultContent,
		IsFollowing:       value.IsFollowing,
		Cover:             ToImageResourceDTO(value.Cover),
		PlanFee:           value.PlanFee,
		HasSupportingPlan: value.HasSupportingPlan,
	}
}

// ToPostImageBlockDTO converts an image block to a DTO.
func ToPostImageBlockDTO(value PostImageBlock) PostImageBlockDTO {
	return PostImageBlockDTO{Resource: sdk.ToResourceDTO(value.Resource), Caption: value.Caption}
}

// ToPostFileBlockDTO converts a file block to a DTO.
func ToPostFileBlockDTO(value PostFileBlock) PostFileBlockDTO {
	return PostFileBlockDTO{Resource: sdk.ToResourceDTO(value.Resource), Name: value.Name, Caption: value.Caption}
}

// ToPostArticleBlockDTO converts an article block to a DTO.
func ToPostArticleBlockDTO(value PostArticleBlock) PostArticleBlockDTO {
	return PostArticleBlockDTO{Text: value.Text}
}

// ToPostVideoEmbedDTO converts a video embed and copies its metadata map.
func ToPostVideoEmbedDTO(value PostVideoEmbed) PostVideoEmbedDTO {
	return PostVideoEmbedDTO{
		Provider:     value.Provider,
		ContentID:    value.ContentID,
		CanonicalURL: value.CanonicalURL,
		Title:        value.Title,
		ThumbnailURL: value.ThumbnailURL,
		VideoID:      value.VideoID,
		EmbeddedData: cloneStringMap(value.EmbeddedData),
	}
}

// ToPostUnknownBlockDTO converts an unknown block and copies its payload.
func ToPostUnknownBlockDTO(value PostUnknownBlock) PostUnknownBlockDTO {
	return PostUnknownBlockDTO{RawType: value.RawType, Payload: cloneStringMap(value.Payload)}
}

// ToPostBlockDTO converts a post block and its optional variants.
func ToPostBlockDTO(value PostBlock) PostBlockDTO {
	var image *PostImageBlockDTO
	if value.Image != nil {
		dto := ToPostImageBlockDTO(*value.Image)
		image = &dto
	}
	var file *PostFileBlockDTO
	if value.File != nil {
		dto := ToPostFileBlockDTO(*value.File)
		file = &dto
	}
	var article *PostArticleBlockDTO
	if value.Article != nil {
		dto := ToPostArticleBlockDTO(*value.Article)
		article = &dto
	}
	var video *PostVideoEmbedDTO
	if value.Video != nil {
		dto := ToPostVideoEmbedDTO(*value.Video)
		video = &dto
	}
	var unknown *PostUnknownBlockDTO
	if value.Unknown != nil {
		dto := ToPostUnknownBlockDTO(*value.Unknown)
		unknown = &dto
	}
	return PostBlockDTO{Kind: value.Kind, Image: image, File: file, Article: article, Video: video, Unknown: unknown}
}

// ToAssetDTO converts an asset to a DTO.
func ToAssetDTO(value Asset) AssetDTO {
	return AssetDTO{
		ID:        value.ID,
		Kind:      value.Kind,
		Name:      value.Name,
		Resource:  sdk.ToResourceDTO(value.Resource),
		Thumbnail: ToImageResourceDTO(value.Thumbnail),
	}
}

// ToPostBodyDTO converts a post body while copying all mutable collections.
func ToPostBodyDTO(value PostBody) PostBodyDTO {
	blocks := make([]PostBlockDTO, 0, len(value.Blocks))
	for _, block := range value.Blocks {
		blocks = append(blocks, ToPostBlockDTO(block))
	}
	assets := make([]AssetDTO, 0, len(value.Assets))
	for _, asset := range value.Assets {
		assets = append(assets, ToAssetDTO(asset))
	}
	return PostBodyDTO{Text: value.Text, Blocks: blocks, Assets: assets}
}

// ToPostDTO converts a post field by field to an output-safe DTO.
func ToPostDTO(value Post) PostDTO {
	var body *PostBodyDTO
	if value.Body != nil {
		dto := ToPostBodyDTO(*value.Body)
		body = &dto
	}
	return PostDTO{
		ID:            value.ID,
		Title:         value.Title,
		PublishedAt:   value.PublishedAt,
		CreatorID:     value.CreatorID,
		FeeRequired:   value.FeeRequired,
		IsRestricted:  value.IsRestricted,
		IsPinned:      value.IsPinned,
		RestrictedFor: value.RestrictedFor,
		CommentCount:  value.CommentCount,
		Cover:         ToImageResourceDTO(value.Cover),
		Body:          body,
	}
}

// ToUserDTO converts a FANBOX user to a DTO.
func ToUserDTO(value User) UserDTO {
	return UserDTO{
		UserID:        value.UserID,
		DisplayName:   value.DisplayName,
		CreatorID:     value.CreatorID,
		CreatorStatus: value.CreatorStatus,
		IsCreator:     value.IsCreator,
	}
}

// ToCreatorTagDTO converts a creator tag to a DTO.
func ToCreatorTagDTO(value CreatorTag) CreatorTagDTO {
	return CreatorTagDTO{Name: value.Name, URL: value.URL}
}

func cloneStringMap(value map[string]string) map[string]string {
	out := make(map[string]string, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}
