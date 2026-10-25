// FANBOX 文章操作：单篇详情、home/supporting 信息流，以及创作者文章列表。
// 请求类型、响应结构与映射在本文件；游标编解码在 cursor.go。

package fanbox

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/fanboxapi"
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

// CreatorPostsRequest lists one creator's posts. Repeat the original CreatorID
// when continuing with Cursor.
type CreatorPostsRequest struct {
	CreatorID string
	Cursor    sdk.Cursor
}

// TaggedPostsRequest lists one creator's posts for a single tag.
type TaggedPostsRequest struct {
	CreatorID string
	Tag       string
	Cursor    sdk.Cursor
}

// PostRequest selects one post by its stable ID.
type PostRequest struct {
	PostID string
}

// HomeRequest lists the current user's home feed.
type HomeRequest struct {
	Cursor sdk.Cursor
}

// SupportingRequest lists posts from the current user's supporting creators.
type SupportingRequest struct {
	Cursor sdk.Cursor
}

// Post returns one post by its stable ID.
func (c *Client) Post(ctx context.Context, request PostRequest) (Post, error) {
	ctx = fanboxapi.WithOperation(ctx, "Post")
	if request.PostID == "" {
		return Post{}, newError("Post", sdk.InvalidArgument, errors.New("post id is required"))
	}
	source, err := c.postInfo(ctx, request.PostID)
	if err != nil {
		return Post{}, classifyError("Post", err)
	}
	post, err := c.mapPost(source)
	if err != nil {
		return Post{}, classifyResponseError("Post", err)
	}
	return post, nil
}

// postInfo fetches the raw post.info response; resource re-resolution reads the
// same body without building resources a second time.
func (c *Client) postInfo(ctx context.Context, postID string) (postWire, error) {
	postID = strings.TrimSpace(postID)
	if postID == "" {
		return postWire{}, errors.New("FANBOX post id is required")
	}
	var response struct {
		Body postInfoWire `json:"body"`
	}
	query := url.Values{fanboxapi.ParamPostID: {postID}}
	if err := c.app.GetJSON(ctx, fanboxapi.PathPostInfo, query, &response); err != nil {
		return postWire{}, err
	}
	return response.Body.Post, nil
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
// Assets list every image before every file, each id once; an explicit list keeps
// its upstream duplicates, as before. Block ImageID/FileID are collected
// regardless of block type; Blocks itself still follows the type.
func (c *Client) mapPostBody(source postWire) (*PostBody, error) {
	wire := source.Body
	body := &PostBody{Text: wire.Text, Blocks: []PostBlock{}, Assets: []Asset{}}
	images, files, err := bodyMedia(wire)
	if err != nil {
		return nil, err
	}
	imageByID := make(map[string]imageWire, len(images))
	for _, image := range images {
		asset, err := c.imageAsset(source, image)
		if err != nil {
			return nil, err
		}
		imageByID[image.ID] = image
		body.Assets = append(body.Assets, asset)
	}
	fileByID := make(map[string]fileWire, len(files))
	for _, file := range files {
		asset, err := c.fileAsset(source, file)
		if err != nil {
			return nil, err
		}
		fileByID[file.ID] = file
		body.Assets = append(body.Assets, asset)
	}
	if wire.Blocks == nil {
		return body, nil
	}
	for _, block := range *wire.Blocks {
		switch PostBlockKind(block.Type) {
		case PostBlockImage:
			if block.ImageID == nil {
				continue
			}
			if image, ok := imageByID[*block.ImageID]; ok && image.OriginalURL != "" {
				res, err := c.newResource("post_image", source.CreatorID, source.ID, image.ID, image.OriginalURL, "")
				if err != nil {
					return nil, err
				}
				body.Blocks = append(body.Blocks, PostBlock{Kind: PostBlockImage, Image: &PostImageBlock{Resource: res}})
			}
		case PostBlockFile:
			if block.FileID == nil {
				continue
			}
			if file, ok := fileByID[*block.FileID]; ok && file.URL != "" {
				res, err := c.newResource("post_file", source.CreatorID, source.ID, file.ID, file.URL, attachmentFilename(file.Name, file.Extension))
				if err != nil {
					return nil, err
				}
				body.Blocks = append(body.Blocks, PostBlock{Kind: PostBlockFile, File: &PostFileBlock{Resource: res, Name: file.Name}})
			}
		case PostBlockArticle:
			body.Blocks = append(body.Blocks, PostBlock{Kind: PostBlockArticle, Article: &PostArticleBlock{}})
		case PostBlockVideo:
			body.Blocks = append(body.Blocks, PostBlock{Kind: PostBlockVideo, Video: &PostVideoEmbed{EmbeddedData: map[string]string{}}})
		default:
			body.Blocks = append(body.Blocks, PostBlock{Kind: PostBlockUnknown, Unknown: &PostUnknownBlock{RawType: block.Type, Payload: map[string]string{}}})
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

// bodyMedia returns the image and file locators of one body variant: the
// explicit list when present, otherwise the block-referenced media in first-use
// order, deduplicated by id.
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
	if err := fanboxapi.ValidateMediaURL(source.OriginalURL); err != nil {
		return err
	}
	if source.ThumbnailURL != "" {
		return fanboxapi.ValidateMediaURL(source.ThumbnailURL)
	}
	return nil
}

func validateFile(source fileWire) error {
	if strings.TrimSpace(source.ID) == "" {
		return errors.New("FANBOX file has no id")
	}
	return fanboxapi.ValidateMediaURL(source.URL)
}

// CreatorPosts lists one creator's posts.
func (c *Client) CreatorPosts(ctx context.Context, request CreatorPostsRequest) (sdk.Page[Post], error) {
	ctx = fanboxapi.WithOperation(ctx, "CreatorPosts")
	if request.CreatorID == "" {
		return sdk.Page[Post]{}, newError("CreatorPosts", sdk.InvalidArgument, errors.New("creator id is required"))
	}
	creatorID := strings.TrimSpace(request.CreatorID)
	if creatorID == "" {
		return sdk.Page[Post]{}, classifyError("CreatorPosts", errors.New("FANBOX creator id is required"))
	}
	cursorQuery := url.Values{fanboxapi.ParamCreatorID: {request.CreatorID}}
	query := url.Values{fanboxapi.ParamCreatorID: {creatorID}}
	return c.postList(ctx, "CreatorPosts", fanboxapi.PathPostListCreator, cursorQuery, query, true, request.Cursor)
}

// TaggedPosts lists one creator's posts for a single tag.
func (c *Client) TaggedPosts(ctx context.Context, request TaggedPostsRequest) (sdk.Page[Post], error) {
	ctx = fanboxapi.WithOperation(ctx, "TaggedPosts")
	if request.CreatorID == "" || request.Tag == "" {
		return sdk.Page[Post]{}, newError("TaggedPosts", sdk.InvalidArgument, errors.New("creator id and tag are required"))
	}
	creatorID := strings.TrimSpace(request.CreatorID)
	if creatorID == "" {
		return sdk.Page[Post]{}, classifyError("TaggedPosts", errors.New("FANBOX creator id is required"))
	}
	tag := strings.TrimSpace(request.Tag)
	if tag == "" {
		return sdk.Page[Post]{}, classifyError("TaggedPosts", errors.New("FANBOX tag is required"))
	}
	cursorQuery := url.Values{fanboxapi.ParamCreatorID: {request.CreatorID}, fanboxapi.ParamTag: {request.Tag}}
	query := url.Values{fanboxapi.ParamCreatorID: {creatorID}, fanboxapi.ParamTag: {tag}}
	return c.postList(ctx, "TaggedPosts", fanboxapi.PathPostListTagged, cursorQuery, query, false, request.Cursor)
}

// Home lists the current user's home feed.
func (c *Client) Home(ctx context.Context, request HomeRequest) (sdk.Page[Post], error) {
	ctx = fanboxapi.WithOperation(ctx, "Home")
	return c.postList(ctx, "Home", fanboxapi.PathPostListHome, nil, nil, true, request.Cursor)
}

// Supporting lists posts from the current user's supporting creators.
func (c *Client) Supporting(ctx context.Context, request SupportingRequest) (sdk.Page[Post], error) {
	ctx = fanboxapi.WithOperation(ctx, "Supporting")
	return c.postList(ctx, "Supporting", fanboxapi.PathPostListSupporting, nil, nil, true, request.Cursor)
}

// postList fetches one post page. cursorQuery binds the cursor and query is sent
// on the first page; pages carrying a server continuation use it verbatim.
// First pages add the explicit upstream page limit when requested. The two query
// values differ when a request carries surrounding whitespace: the digest keeps
// the caller's value so existing cursors stay valid, while the wire sees it trimmed.
func (c *Client) postList(ctx context.Context, operation, path string, cursorQuery, query url.Values, limit bool, cursor sdk.Cursor) (sdk.Page[Post], error) {
	nextURL, err := c.continuationURL(ctx, operation, cursorQuery, cursor)
	if err != nil {
		return sdk.Page[Post]{}, err
	}
	if limit && strings.TrimSpace(nextURL) == "" {
		query = cloneValues(query)
		query.Set(fanboxapi.ParamLimit, strconv.Itoa(fanboxapi.PostListPageLimit))
	}
	var body json.RawMessage
	acceptItems := path == fanboxapi.PathPostListHome || path == fanboxapi.PathPostListSupporting
	if err := c.getListBody(ctx, path, query, nextURL, &body, operation); err != nil {
		return sdk.Page[Post]{}, err
	}
	var page postPageWire
	if err := json.Unmarshal(body, &page); err != nil {
		return sdk.Page[Post]{}, classifyError(operation, errors.New("decode FANBOX post list response"))
	}
	wirePosts := page.Posts
	if acceptItems && wirePosts == nil {
		wirePosts = page.Items
	}
	if wirePosts == nil {
		return sdk.Page[Post]{}, classifyError(operation, errors.New("FANBOX post list is missing items"))
	}
	items := make([]Post, 0, len(wirePosts))
	for _, source := range wirePosts {
		post, err := c.mapPost(source)
		if err != nil {
			return sdk.Page[Post]{}, classifyResponseError(operation, err)
		}
		items = append(items, post)
	}
	next, err := c.buildCursor(ctx, operation, cursorQuery, firstContinuation(page.PageURLs, page.NextURL))
	if err != nil {
		return sdk.Page[Post]{}, err
	}
	return sdk.Page[Post]{Items: items, Next: next}, nil
}

func cloneValues(source url.Values) url.Values {
	clone := make(url.Values, len(source))
	for key, values := range source {
		clone[key] = append([]string(nil), values...)
	}
	return clone
}
