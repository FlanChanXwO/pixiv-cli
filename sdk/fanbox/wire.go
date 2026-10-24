// FANBOX 响应结构、字段映射与同族端点的共同解码。
// 公开实体在 models.go，HTTP 路由和请求参数在 creator.go、post.go。

package fanbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	protocol "github.com/FlanChanXwO/pixiv-cli/internal/fanboxapi"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// postInfoWire is the post.info body envelope.
type postInfoWire struct {
	Post postWire `json:"post"`
}

// postPageWire is the post-list body shared by home, supporting, creator and
// tagged routes. Home/supporting use items; creator/tagged lists use posts.
type postPageWire struct {
	Posts    []postWire `json:"posts"`
	Items    []postWire `json:"items"`
	NextURL  string     `json:"nextUrl"`
	PageURLs []string   `json:"pageUrls"`
}

type postWire struct {
	ID                string        `json:"id"`
	Title             string        `json:"title"`
	PublishedDateTime string        `json:"publishedDatetime"`
	CreatorID         string        `json:"creatorId"`
	FeeRequired       int           `json:"feeRequired"`
	IsRestricted      bool          `json:"isRestricted"`
	IsPinned          bool          `json:"isPinned"`
	RestrictedFor     int           `json:"restrictedFor"`
	CommentCount      int           `json:"commentCount"`
	Body              *postBodyWire `json:"body"`
}

type postBodyWire struct {
	Text     string               `json:"text"`
	Files    *[]fileWire          `json:"files"`
	Images   *[]imageWire         `json:"images"`
	Blocks   *[]blockWire         `json:"blocks"`
	ImageMap map[string]imageWire `json:"imageMap"`
	FileMap  map[string]fileWire  `json:"fileMap"`
}

type blockWire struct {
	Type    string  `json:"type"`
	ImageID *string `json:"imageId"`
	FileID  *string `json:"fileId"`
}

type imageWire struct {
	ID           string `json:"id"`
	Extension    string `json:"extension"`
	OriginalURL  string `json:"originalUrl"`
	ThumbnailURL string `json:"thumbnailUrl"`
}

type fileWire struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Extension string `json:"extension"`
	URL       string `json:"url"`
}

type creatorProfileWire struct {
	CreatorID string `json:"creatorId"`
	User      struct {
		Name    string `json:"name"`
		IconURL string `json:"iconUrl"`
	} `json:"user"`
	HasAdultContent bool   `json:"hasAdultContent"`
	IsFollowing     bool   `json:"isFollowing"`
	CoverImageURL   string `json:"coverImageUrl"`
	Plan            struct {
		Fee               int  `json:"fee"`
		HasSupportingPlan bool `json:"hasSupportingPlan"`
	} `json:"plan"`
}

type creatorSummaryWire struct {
	CreatorID string `json:"creatorId"`
}

type creatorPageWire struct {
	Plans    *[]creatorSummaryWire `json:"plans"`
	Creators *[]creatorSummaryWire `json:"creators"`
	NextURL  string                `json:"nextUrl"`
	PageURLs []string              `json:"pageUrls"`
}

type tagWire struct {
	Tag string `json:"tag"`
	URL string `json:"url"`
}

// mapPost converts one post response into the public model.
func (c *Client) mapPost(source postWire) (Post, error) {
	if strings.TrimSpace(source.ID) == "" {
		return Post{}, errors.New("FANBOX post has no id")
	}
	published, err := parseUTCTime(source.PublishedDateTime)
	if err != nil {
		return Post{}, newError("Post", sdk.MalformedUpstreamResponse, err)
	}
	out := Post{
		ID:            source.ID,
		Title:         source.Title,
		PublishedAt:   published,
		CreatorID:     source.CreatorID,
		FeeRequired:   source.FeeRequired,
		IsRestricted:  source.IsRestricted,
		IsPinned:      source.IsPinned,
		RestrictedFor: source.RestrictedFor,
		CommentCount:  source.CommentCount,
	}
	if source.Body == nil {
		return out, nil
	}
	body, err := c.mapPostBody(source)
	if err != nil {
		return Post{}, err
	}
	out.Body = body
	return out, nil
}

// mapPostBody maps the one body variant the response provides: the images
// list, the files list, or ordered blocks whose media live in imageMap/fileMap.
// Assets are flattened in upstream display order; the block variant appends one
// asset per referenced media.
func (c *Client) mapPostBody(source postWire) (*PostBody, error) {
	wire := source.Body
	body := &PostBody{Text: wire.Text, Blocks: []PostBlock{}, Assets: []Asset{}}
	switch {
	case wire.Images != nil:
		for _, image := range *wire.Images {
			asset, err := c.imageAsset(source, image)
			if err != nil {
				return nil, err
			}
			body.Assets = append(body.Assets, asset)
		}
	case wire.Files != nil:
		for _, file := range *wire.Files {
			asset, err := c.fileAsset(source, file)
			if err != nil {
				return nil, err
			}
			body.Assets = append(body.Assets, asset)
		}
	case wire.Blocks != nil:
		for _, block := range *wire.Blocks {
			switch PostBlockKind(block.Type) {
			case PostBlockImage:
				if block.ImageID == nil {
					continue
				}
				image, found := wire.ImageMap[*block.ImageID]
				if !found {
					return nil, errors.New("FANBOX blog block references a missing image")
				}
				asset, err := c.imageAsset(source, image)
				if err != nil {
					return nil, err
				}
				body.Assets = append(body.Assets, asset)
				body.Blocks = append(body.Blocks, PostBlock{Kind: PostBlockImage, Image: &PostImageBlock{Resource: asset.Resource}})
			case PostBlockFile:
				if block.FileID == nil {
					continue
				}
				file, found := wire.FileMap[*block.FileID]
				if !found {
					return nil, errors.New("FANBOX blog block references a missing file")
				}
				asset, err := c.fileAsset(source, file)
				if err != nil {
					return nil, err
				}
				body.Assets = append(body.Assets, asset)
				body.Blocks = append(body.Blocks, PostBlock{Kind: PostBlockFile, File: &PostFileBlock{Resource: asset.Resource, Name: asset.Name}})
			case PostBlockArticle:
				body.Blocks = append(body.Blocks, PostBlock{Kind: PostBlockArticle, Article: &PostArticleBlock{}})
			case PostBlockVideo:
				body.Blocks = append(body.Blocks, PostBlock{Kind: PostBlockVideo, Video: &PostVideoEmbed{EmbeddedData: map[string]string{}}})
			default:
				body.Blocks = append(body.Blocks, PostBlock{Kind: PostBlockUnknown, Unknown: &PostUnknownBlock{RawType: block.Type, Payload: map[string]string{}}})
			}
		}
	}
	return body, nil
}

func (c *Client) imageAsset(source postWire, image imageWire) (Asset, error) {
	if err := validateImage(image); err != nil {
		return Asset{}, err
	}
	res, err := c.newResource("post_image", source.CreatorID, source.ID, image.ID, image.OriginalURL, "")
	if err != nil {
		return Asset{}, err
	}
	return Asset{ID: image.ID, Kind: AssetKindImage, Resource: res}, nil
}

func (c *Client) fileAsset(source postWire, file fileWire) (Asset, error) {
	if err := validateFile(file); err != nil {
		return Asset{}, err
	}
	res, err := c.newResource("post_file", source.CreatorID, source.ID, file.ID, file.URL, attachmentFilename(file.Name, file.Extension))
	if err != nil {
		return Asset{}, err
	}
	return Asset{ID: file.ID, Kind: AssetKindFile, Name: file.Name, Resource: res}, nil
}

// bodyMedia returns the image and file locators of one body variant for
// resource re-resolution: the explicit list when present, otherwise the media
// referenced by blocks, each id once in first-use order.
func bodyMedia(body *postBodyWire) ([]imageWire, []fileWire, error) {
	switch {
	case body.Images != nil:
		for _, image := range *body.Images {
			if err := validateImage(image); err != nil {
				return nil, nil, err
			}
		}
		return *body.Images, nil, nil
	case body.Files != nil:
		for _, file := range *body.Files {
			if err := validateFile(file); err != nil {
				return nil, nil, err
			}
		}
		return nil, *body.Files, nil
	case body.Blocks != nil:
		return blockMedia(body)
	default:
		return nil, nil, nil
	}
}

func blockMedia(body *postBodyWire) ([]imageWire, []fileWire, error) {
	var images []imageWire
	var files []fileWire
	seenImages := map[string]bool{}
	seenFiles := map[string]bool{}
	for _, block := range *body.Blocks {
		if block.ImageID != nil && !seenImages[*block.ImageID] {
			image, found := body.ImageMap[*block.ImageID]
			if !found {
				return nil, nil, errors.New("FANBOX blog block references a missing image")
			}
			if err := validateImage(image); err != nil {
				return nil, nil, err
			}
			seenImages[*block.ImageID] = true
			images = append(images, image)
		}
		if block.FileID != nil && !seenFiles[*block.FileID] {
			file, found := body.FileMap[*block.FileID]
			if !found {
				return nil, nil, errors.New("FANBOX blog block references a missing file")
			}
			if err := validateFile(file); err != nil {
				return nil, nil, err
			}
			seenFiles[*block.FileID] = true
			files = append(files, file)
		}
	}
	return images, files, nil
}

func validateImage(source imageWire) error {
	if strings.TrimSpace(source.ID) == "" {
		return errors.New("FANBOX image has no id")
	}
	if err := protocol.ValidateMediaURL(source.OriginalURL); err != nil {
		return err
	}
	if source.ThumbnailURL != "" {
		return protocol.ValidateMediaURL(source.ThumbnailURL)
	}
	return nil
}

func validateFile(source fileWire) error {
	if strings.TrimSpace(source.ID) == "" {
		return errors.New("FANBOX file has no id")
	}
	return protocol.ValidateMediaURL(source.URL)
}

// decodeCreatorSummaries accepts either a bare array or the plans/creators
// envelope used by the supporting and following creator lists.
func decodeCreatorSummaries(raw json.RawMessage, kind CreatorListKind) ([]CreatorSummary, string, error) {
	var direct []creatorSummaryWire
	if err := json.Unmarshal(raw, &direct); err == nil {
		items, err := mapCreatorSummaries(direct)
		return items, "", err
	}
	var wrapped creatorPageWire
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, "", fmt.Errorf("decode FANBOX %s creators", kind)
	}
	var source *[]creatorSummaryWire
	switch kind {
	case CreatorListSupporting:
		source = wrapped.Plans
	case CreatorListFollowing:
		source = wrapped.Creators
	default:
		return nil, "", errors.New("FANBOX creator list kind is invalid")
	}
	if source == nil {
		return nil, "", fmt.Errorf("decode FANBOX %s creators", kind)
	}
	items, err := mapCreatorSummaries(*source)
	if err != nil {
		return nil, "", err
	}
	return items, firstContinuation(wrapped.PageURLs, wrapped.NextURL), nil
}

func mapCreatorSummaries(items []creatorSummaryWire) ([]CreatorSummary, error) {
	result := make([]CreatorSummary, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.CreatorID)
		if id == "" {
			return nil, errors.New("FANBOX creator response includes an empty creator id")
		}
		result = append(result, CreatorSummary{ID: id})
	}
	return result, nil
}

// decodeFeaturedTags accepts either a bare array or the tags envelope.
func decodeFeaturedTags(raw json.RawMessage) ([]CreatorTag, error) {
	var direct []tagWire
	if err := json.Unmarshal(raw, &direct); err == nil {
		return mapTags(direct)
	}
	var wrapped struct {
		Tags []tagWire `json:"tags"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, errors.New("decode FANBOX creator tags")
	}
	return mapTags(wrapped.Tags)
}

func mapTags(items []tagWire) ([]CreatorTag, error) {
	result := make([]CreatorTag, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Tag)
		if name == "" {
			return nil, errors.New("FANBOX creator tag response includes an empty tag")
		}
		result = append(result, CreatorTag{Name: name, URL: item.URL})
	}
	return result, nil
}

func mapUser(identity protocol.Identity) User {
	return User{
		UserID:        identity.UserID,
		DisplayName:   identity.DisplayName,
		CreatorID:     identity.CreatorID,
		CreatorStatus: identity.CreatorStatus,
		IsCreator:     identity.IsCreator,
	}
}

func parseUTCTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, errors.New("no publish time")
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

// firstContinuation prefers the server pageUrls entry over nextUrl.
func firstContinuation(pageURLs []string, nextURL string) string {
	if len(pageURLs) > 0 {
		return pageURLs[0]
	}
	return nextURL
}
