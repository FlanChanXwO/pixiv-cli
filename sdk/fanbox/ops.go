package fanbox

import (
	"context"
	"errors"
	"net/url"
	"time"

	creatorservice "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/creator"
	creatorlist "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/creator/creators"
	creatortags "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/creator/tags"
	postservice "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post/home"
	postinfo "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post/info"
	postposts "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post/posts"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post/supporting"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/protocol"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// ValidateSession verifies the current session identity. An expired session
// returns CredentialsExpired.
func (c *Client) ValidateSession(ctx context.Context) error {
	_, err := c.session.CurrentUser(ctx)
	if err != nil {
		return classifyError("ValidateSession", err)
	}
	return nil
}

// CurrentUser returns the current authenticated user's identity.
func (c *Client) CurrentUser(ctx context.Context, request CurrentUserRequest) (User, error) {
	identity, err := c.session.CurrentUser(ctx)
	if err != nil {
		return User{}, classifyError("CurrentUser", err)
	}
	return mapUser(identity), nil
}

// Creator returns one creator's profile.
func (c *Client) Creator(ctx context.Context, request CreatorRequest) (Creator, error) {
	if request.CreatorID == "" {
		return Creator{}, newError("Creator", sdk.InvalidArgument, errors.New("creator id is required"))
	}
	profile, err := c.creators.Profile(ctx, creatorlist.ProfileRequest{CreatorID: request.CreatorID})
	if err != nil {
		return Creator{}, classifyError("Creator", err)
	}
	return c.mapCreator(profile)
}

// Creators lists supporting or following creators.
func (c *Client) Creators(ctx context.Context, request CreatorsRequest) (sdk.Page[CreatorSummary], error) {
	kind := request.Kind
	if kind == "" {
		kind = CreatorListSupporting
	}
	query := url.Values{"kind": {string(kind)}}
	nextURL, err := c.continuationURL("Creators", query, request.Cursor)
	if err != nil {
		return sdk.Page[CreatorSummary]{}, err
	}
	page, err := c.creators.List(ctx, creatorlist.ListRequest{Kind: creatorlist.ListKind(kind), NextURL: nextURL})
	if err != nil {
		return sdk.Page[CreatorSummary]{}, classifyError("Creators", err)
	}
	items := make([]CreatorSummary, 0, len(page.Items))
	for _, creator := range page.Items {
		items = append(items, c.mapCreatorSummary(creator))
	}
	next, err := c.buildCursor("Creators", query, page.NextURL)
	if err != nil {
		return sdk.Page[CreatorSummary]{}, err
	}
	return sdk.Page[CreatorSummary]{Items: items, Next: next}, nil
}

// CreatorTags lists the tags used by one creator.
func (c *Client) CreatorTags(ctx context.Context, request CreatorTagsRequest) ([]CreatorTag, error) {
	if request.CreatorID == "" {
		return nil, newError("CreatorTags", sdk.InvalidArgument, errors.New("creator id is required"))
	}
	tags, err := c.creatorTags.List(ctx, creatortags.Request{CreatorID: request.CreatorID})
	if err != nil {
		return nil, classifyError("CreatorTags", err)
	}
	items := make([]CreatorTag, 0, len(tags))
	for _, tag := range tags {
		items = append(items, CreatorTag{Name: tag.Name, URL: tag.URL})
	}
	return items, nil
}

// CreatorPosts lists one creator's posts.
func (c *Client) CreatorPosts(ctx context.Context, request CreatorPostsRequest) (sdk.Page[Post], error) {
	if request.CreatorID == "" {
		return sdk.Page[Post]{}, newError("CreatorPosts", sdk.InvalidArgument, errors.New("creator id is required"))
	}
	query := url.Values{"creatorId": {request.CreatorID}}
	nextURL, err := c.continuationURL("CreatorPosts", query, request.Cursor)
	if err != nil {
		return sdk.Page[Post]{}, err
	}
	page, err := c.creatorPosts.Creator(ctx, postposts.Request{CreatorID: request.CreatorID, NextURL: nextURL})
	if err != nil {
		return sdk.Page[Post]{}, classifyError("CreatorPosts", err)
	}
	return c.postPage("CreatorPosts", query, page)
}

// TaggedPosts lists one creator's posts for a single tag.
func (c *Client) TaggedPosts(ctx context.Context, request TaggedPostsRequest) (sdk.Page[Post], error) {
	if request.CreatorID == "" || request.Tag == "" {
		return sdk.Page[Post]{}, newError("TaggedPosts", sdk.InvalidArgument, errors.New("creator id and tag are required"))
	}
	query := url.Values{"creatorId": {request.CreatorID}, "tag": {request.Tag}}
	nextURL, err := c.continuationURL("TaggedPosts", query, request.Cursor)
	if err != nil {
		return sdk.Page[Post]{}, err
	}
	page, err := c.creatorPosts.Tagged(ctx, postposts.Request{CreatorID: request.CreatorID, Tag: request.Tag, NextURL: nextURL})
	if err != nil {
		return sdk.Page[Post]{}, classifyError("TaggedPosts", err)
	}
	return c.postPage("TaggedPosts", query, page)
}

// Post returns one post by its stable ID.
func (c *Client) Post(ctx context.Context, request PostRequest) (Post, error) {
	if request.PostID == "" {
		return Post{}, newError("Post", sdk.InvalidArgument, errors.New("post id is required"))
	}
	post, err := c.postInfo.Get(ctx, postinfo.Request{PostID: request.PostID})
	if err != nil {
		return Post{}, classifyError("Post", err)
	}
	return c.mapPost(post)
}

// Home lists the current user's home feed.
func (c *Client) Home(ctx context.Context, request HomeRequest) (sdk.Page[Post], error) {
	query := url.Values{}
	nextURL, err := c.continuationURL("Home", query, request.Cursor)
	if err != nil {
		return sdk.Page[Post]{}, err
	}
	page, err := c.home.List(ctx, home.Request{NextURL: nextURL})
	if err != nil {
		return sdk.Page[Post]{}, classifyError("Home", err)
	}
	return c.postPage("Home", query, page)
}

// Supporting lists posts from the current user's supporting creators.
func (c *Client) Supporting(ctx context.Context, request SupportingRequest) (sdk.Page[Post], error) {
	query := url.Values{}
	nextURL, err := c.continuationURL("Supporting", query, request.Cursor)
	if err != nil {
		return sdk.Page[Post]{}, err
	}
	page, err := c.supporting.List(ctx, supporting.Request{NextURL: nextURL})
	if err != nil {
		return sdk.Page[Post]{}, classifyError("Supporting", err)
	}
	return c.postPage("Supporting", query, page)
}

func (c *Client) postPage(op string, query url.Values, page postservice.Page) (sdk.Page[Post], error) {
	items := make([]Post, 0, len(page.Posts))
	for _, source := range page.Posts {
		post, err := c.mapPost(source)
		if err != nil {
			return sdk.Page[Post]{}, err
		}
		items = append(items, post)
	}
	next, err := c.buildCursor(op, query, page.NextURL)
	if err != nil {
		return sdk.Page[Post]{}, err
	}
	return sdk.Page[Post]{Items: items, Next: next}, nil
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

func (c *Client) mapCreatorSummary(source creatorservice.Summary) CreatorSummary {
	out := CreatorSummary{ID: source.ID}
	return out
}

func (c *Client) mapCreator(profile creatorservice.Creator) (Creator, error) {
	out := Creator{
		CreatorSummary:    CreatorSummary{ID: profile.ID, Name: profile.DisplayName},
		HasAdultContent:   profile.HasAdultContent,
		IsFollowing:       profile.IsFollowing,
		PlanFee:           profile.PlanFee,
		HasSupportingPlan: profile.HasSupportingPlan,
	}
	if profile.IconURL != "" {
		res, err := c.newResource("creator_icon", profile.ID, profile.IconURL)
		if err != nil {
			return Creator{}, err
		}
		out.Icon = ImageResource{Resource: res}
	}
	if profile.CoverURL != "" {
		res, err := c.newResource("creator_cover", profile.ID, profile.CoverURL)
		if err != nil {
			return Creator{}, err
		}
		out.Cover = ImageResource{Resource: res}
	}
	return out, nil
}

func (c *Client) mapPost(source postservice.Post) (Post, error) {
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
	body := &PostBody{Text: source.Body.Text}
	imageByID := map[string]postservice.Image{}
	fileByID := map[string]postservice.File{}
	for _, image := range mergePostImages(*source.Body) {
		imageByID[image.ID] = image
		if image.OriginalURL != "" {
			res, err := c.newResource("post_image", image.ID, image.OriginalURL)
			if err != nil {
				return Post{}, err
			}
			body.Assets = append(body.Assets, Asset{ID: image.ID, Kind: AssetKindImage, Resource: res})
		}
	}
	for _, file := range mergePostFiles(*source.Body) {
		fileByID[file.ID] = file
		if file.URL != "" {
			res, err := c.newResource("post_file", file.ID, file.URL)
			if err != nil {
				return Post{}, err
			}
			body.Assets = append(body.Assets, Asset{ID: file.ID, Kind: AssetKindFile, Name: file.Name, Resource: res})
		}
	}
	for _, block := range source.Body.Blocks {
		switch block.Type {
		case "image":
			if image, ok := imageByID[block.ImageID]; ok && image.OriginalURL != "" {
				res, err := c.newResource("post_image", image.ID, image.OriginalURL)
				if err != nil {
					return Post{}, err
				}
				body.Blocks = append(body.Blocks, PostBlock{Kind: PostBlockImage, Image: &PostImageBlock{Resource: res}})
			}
		case "file":
			if file, ok := fileByID[block.FileID]; ok && file.URL != "" {
				res, err := c.newResource("post_file", file.ID, file.URL)
				if err != nil {
					return Post{}, err
				}
				body.Blocks = append(body.Blocks, PostBlock{Kind: PostBlockFile, File: &PostFileBlock{Resource: res, Name: file.Name}})
			}
		case "article":
			body.Blocks = append(body.Blocks, PostBlock{Kind: PostBlockArticle, Article: &PostArticleBlock{}})
		case "video":
			body.Blocks = append(body.Blocks, PostBlock{Kind: PostBlockVideo, Video: &PostVideoEmbed{}})
		default:
			body.Blocks = append(body.Blocks, PostBlock{Kind: PostBlockUnknown, Unknown: &PostUnknownBlock{RawType: block.Type}})
		}
	}
	out.Body = body
	return out, nil
}

// mergePostImages 补上 FANBOX 在 block 的 imageMap 中提供、但未填充 Images 列表的资源。
func mergePostImages(body postservice.Body) []postservice.Image {
	images := append([]postservice.Image(nil), body.Images...)
	seen := make(map[string]struct{}, len(images))
	for _, image := range images {
		if image.ID != "" {
			seen[image.ID] = struct{}{}
		}
	}
	for _, asset := range body.Assets {
		if asset.Kind != postservice.AssetKindImage || asset.ID == "" {
			continue
		}
		if _, ok := seen[asset.ID]; ok {
			continue
		}
		images = append(images, postservice.Image{
			ID:           asset.ID,
			Extension:    asset.Extension,
			OriginalURL:  asset.URL,
			ThumbnailURL: asset.ThumbnailURL,
		})
		seen[asset.ID] = struct{}{}
	}
	return images
}

// mergePostFiles 补上 FANBOX 在 block 的 fileMap 中提供、但未填充 Files 列表的资源。
func mergePostFiles(body postservice.Body) []postservice.File {
	files := append([]postservice.File(nil), body.Files...)
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		if file.ID != "" {
			seen[file.ID] = struct{}{}
		}
	}
	for _, asset := range body.Assets {
		if asset.Kind != postservice.AssetKindFile || asset.ID == "" {
			continue
		}
		if _, ok := seen[asset.ID]; ok {
			continue
		}
		files = append(files, postservice.File{
			ID:        asset.ID,
			Name:      asset.Name,
			Extension: asset.Extension,
			URL:       asset.URL,
		})
		seen[asset.ID] = struct{}{}
	}
	return files
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
