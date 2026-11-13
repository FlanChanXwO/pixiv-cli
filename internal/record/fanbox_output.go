package record

import (
	"time"

	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

type fanboxImageResource struct {
	Resource legacysdk.Resource `json:"resource"`
	Variant  string             `json:"variant"`
	Width    int                `json:"width"`
	Height   int                `json:"height"`
}

type fanboxCreatorSummary struct {
	ID   string              `json:"id"`
	Name string              `json:"name"`
	Icon fanboxImageResource `json:"icon"`
}

type fanboxPostBlockKind string

const (
	postBlockImage   fanboxPostBlockKind = "image"
	postBlockFile    fanboxPostBlockKind = "file"
	postBlockArticle fanboxPostBlockKind = "article"
	postBlockVideo   fanboxPostBlockKind = "video"
	postBlockUnknown fanboxPostBlockKind = "unknown"
)

type fanboxPostImageBlock struct {
	Resource legacysdk.Resource `json:"resource"`
	Caption  string             `json:"caption"`
}

type fanboxPostFileBlock struct {
	Resource legacysdk.Resource `json:"resource"`
	Name     string             `json:"name"`
	Caption  string             `json:"caption"`
}

type fanboxPostArticleBlock struct {
	Text string `json:"text"`
}

type fanboxPostVideoEmbed struct {
	Provider     string            `json:"provider"`
	ContentID    string            `json:"content_id"`
	CanonicalURL string            `json:"canonical_url"`
	Title        string            `json:"title"`
	ThumbnailURL string            `json:"thumbnail_url"`
	VideoID      string            `json:"video_id"`
	EmbeddedData map[string]string `json:"embedded_data"`
}

type fanboxPostUnknownBlock struct {
	RawType string            `json:"raw_type"`
	Payload map[string]string `json:"payload"`
}

type fanboxPostBlock struct {
	Kind    fanboxPostBlockKind     `json:"kind"`
	Image   *fanboxPostImageBlock   `json:"image"`
	File    *fanboxPostFileBlock    `json:"file"`
	Article *fanboxPostArticleBlock `json:"article"`
	Video   *fanboxPostVideoEmbed   `json:"video"`
	Unknown *fanboxPostUnknownBlock `json:"unknown"`
}

// FanboxAssetOut 保留公开资源引用和资产字段，省略下载 URL。
type FanboxAssetOut struct {
	ID        string              `json:"id"`
	Kind      string              `json:"kind"`
	Name      string              `json:"name"`
	Resource  legacysdk.Resource  `json:"resource"`
	Thumbnail fanboxImageResource `json:"thumbnail"`
}

type fanboxPostBody struct {
	Text   string            `json:"text"`
	Blocks []fanboxPostBlock `json:"blocks"`
	Assets []FanboxAssetOut  `json:"assets"`
}

type fanboxPost struct {
	ID            string              `json:"id"`
	Title         string              `json:"title"`
	PublishedAt   time.Time           `json:"published_at"`
	CreatorID     string              `json:"creator_id"`
	FeeRequired   int                 `json:"fee_required"`
	IsRestricted  bool                `json:"is_restricted"`
	IsPinned      bool                `json:"is_pinned"`
	RestrictedFor int                 `json:"restricted_for"`
	CommentCount  int                 `json:"comment_count"`
	Cover         fanboxImageResource `json:"cover"`
	Body          *fanboxPostBody     `json:"body"`
}

// FanboxCreatorSummary 保留列表的 ID 摘要；上游列表附带的资料不改变既有输出。
func FanboxCreatorSummary(creator *fanbox.Creator) any { return fanboxCreatorSummary{ID: creator.ID} }

// FanboxAssets 保持图片先于附件的顺序；资源身份与正文下载地址分离。
func FanboxAssets(post *fanbox.Post) []FanboxAssetOut {
	assets := []FanboxAssetOut{}
	if post.Body == nil {
		return assets
	}
	for _, image := range post.Body.Images {
		assets = append(assets, FanboxAssetOut{ID: image.ID, Kind: "image", Resource: FanboxImageResource(FanboxPostImage, post.CreatorID, post.ID, image.ID, image.URL)})
	}
	for _, file := range post.Body.Files {
		assets = append(assets, FanboxAssetOut{ID: file.ID, Kind: "file", Name: file.Name, Resource: FanboxImageResource(FanboxPostFile, post.CreatorID, post.ID, file.ID, file.URL)})
	}
	return assets
}

// FanboxPost 保留 CLI 列表的完整外部形状；SDK 正文 Payload 与 URL 留在绑定对象内。
func FanboxPost(post *fanbox.Post) any {
	out := fanboxPost{ID: post.ID, Title: post.Title, PublishedAt: post.PublishedAt, CreatorID: post.CreatorID, FeeRequired: post.FeeRequired, IsRestricted: post.IsRestricted, IsPinned: post.IsPinned, RestrictedFor: post.RestrictedFor, CommentCount: post.CommentCount}
	if post.Body == nil {
		return out
	}
	body := &fanboxPostBody{Text: post.Body.Text, Blocks: []fanboxPostBlock{}, Assets: FanboxAssets(post)}
	for _, block := range post.Body.Blocks {
		outBlock := fanboxPostBlock{}
		switch block.Style {
		case fanbox.BlockImage:
			if block.Image == nil {
				continue
			}
			outBlock.Kind = postBlockImage
			outBlock.Image = &fanboxPostImageBlock{Resource: FanboxImageResource(FanboxPostImage, post.CreatorID, post.ID, block.Image.ID, block.Image.URL)}
		case fanbox.BlockFile:
			if block.File == nil {
				continue
			}
			outBlock.Kind = postBlockFile
			outBlock.File = &fanboxPostFileBlock{Resource: FanboxImageResource(FanboxPostFile, post.CreatorID, post.ID, block.File.ID, block.File.URL), Name: block.File.Name}
		case fanbox.BlockArticle:
			outBlock.Kind = postBlockArticle
			outBlock.Article = &fanboxPostArticleBlock{}
		case fanbox.BlockVideo:
			outBlock.Kind = postBlockVideo
			outBlock.Video = &fanboxPostVideoEmbed{EmbeddedData: map[string]string{}}
		default:
			outBlock.Kind = postBlockUnknown
			outBlock.Unknown = &fanboxPostUnknownBlock{RawType: string(block.Style), Payload: map[string]string{}}
		}
		body.Blocks = append(body.Blocks, outBlock)
	}
	out.Body = body
	return out
}
