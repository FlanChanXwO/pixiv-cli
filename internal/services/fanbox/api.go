package fanbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// CreatorListKind 指定读取支持中的创作者或关注中的创作者。
type CreatorListKind string

const (
	CreatorListSupporting CreatorListKind = "supporting"
	CreatorListFollowing  CreatorListKind = "following"
)

// AssetKind 是 post body 中单个媒体资产的类型。
type AssetKind string

const (
	AssetKindImage AssetKind = "image"
	AssetKindFile  AssetKind = "file"
)

// Identity 是从 FANBOX 网页 metadata 识别出的非敏感登录身份。
type Identity struct {
	UserID        int64  `json:"user_id"`
	DisplayName   string `json:"display_name"`
	CreatorID     string `json:"creator_id,omitempty"`
	CreatorStatus string `json:"creator_status,omitempty"`
	IsCreator     bool   `json:"is_creator"`
}

// Creator 是 FANBOX creator id 的安全摘要。
type Creator struct {
	ID string `json:"id"`
}

// CreatorPage is one creator list page and the server-provided continuation.
type CreatorPage struct {
	Creators []Creator
	NextURL  string
}

// CreatorProfile 是创作者公开摘要；它不含订阅、Cookie 或任何需要导出的私人账户字段。
type CreatorProfile struct {
	ID                string `json:"id"`
	DisplayName       string `json:"display_name,omitempty"`
	IconURL           string `json:"icon_url,omitempty"`
	HasAdultContent   bool   `json:"has_adult_content,omitempty"`
	IsFollowing       bool   `json:"is_following,omitempty"`
	CoverURL          string `json:"cover_url,omitempty"`
	PlanFee           int    `json:"plan_fee,omitempty"`
	HasSupportingPlan bool   `json:"has_supporting_plan,omitempty"`
}

// Post 是 FANBOX post 的公开读取模型；受限 post 的 Body 可为 nil。
type Post struct {
	ID                string    `json:"id"`
	Title             string    `json:"title"`
	PublishedDateTime string    `json:"published_datetime,omitempty"`
	CreatorID         string    `json:"creator_id,omitempty"`
	FeeRequired       int       `json:"fee_required,omitempty"`
	IsRestricted      bool      `json:"is_restricted"`
	IsPinned          bool      `json:"is_pinned"`
	RestrictedFor     int       `json:"restricted_for,omitempty"`
	CommentCount      int       `json:"comment_count,omitempty"`
	Body              *PostBody `json:"body,omitempty"`
}

// PostBody 保留原始 image/file 集合，并提供按 FANBOX 定义顺序展平的 Assets。
type PostBody struct {
	Text   string  `json:"text,omitempty"`
	Images []Image `json:"images,omitempty"`
	Files  []File  `json:"files,omitempty"`
	Blocks []Block `json:"blocks,omitempty"`
	Assets []Asset `json:"assets,omitempty"`
}

// Image 是 FANBOX 图片资产。
type Image struct {
	ID           string `json:"id"`
	Extension    string `json:"extension,omitempty"`
	OriginalURL  string `json:"original_url"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
}

// File 是 FANBOX 文件资产。
type File struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Extension string `json:"extension,omitempty"`
	URL       string `json:"url"`
}

// Block 是 post body 中引用 image/file 地图的富文本块。
type Block struct {
	Type    string `json:"type"`
	ImageID string `json:"image_id,omitempty"`
	FileID  string `json:"file_id,omitempty"`
}

// Asset 是可以由 OpenMedia 读取的媒体描述，不包含认证信息或下载落盘策略。
type Asset struct {
	ID           string    `json:"id"`
	Kind         AssetKind `json:"kind"`
	Name         string    `json:"name,omitempty"`
	Extension    string    `json:"extension,omitempty"`
	URL          string    `json:"url"`
	ThumbnailURL string    `json:"thumbnail_url,omitempty"`
}

// PostPage 是一页 post 列表及其服务端 nextUrl 游标。
type PostPage struct {
	Posts   []Post `json:"posts"`
	NextURL string `json:"next_url,omitempty"`
}

type creatorDTO struct {
	CreatorID string `json:"creatorId"`
}

type creatorProfileDTO struct {
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

type postDTO struct {
	ID                string       `json:"id"`
	Title             string       `json:"title"`
	PublishedDateTime string       `json:"publishedDatetime"`
	CreatorID         string       `json:"creatorId"`
	FeeRequired       int          `json:"feeRequired"`
	IsRestricted      bool         `json:"isRestricted"`
	IsPinned          bool         `json:"isPinned"`
	RestrictedFor     int          `json:"restrictedFor"`
	CommentCount      int          `json:"commentCount"`
	Body              *postBodyDTO `json:"body"`
}

type postBodyDTO struct {
	Text     string              `json:"text"`
	Files    *[]fileDTO          `json:"files"`
	Images   *[]imageDTO         `json:"images"`
	Blocks   *[]blockDTO         `json:"blocks"`
	ImageMap map[string]imageDTO `json:"imageMap"`
	FileMap  map[string]fileDTO  `json:"fileMap"`
}

type blockDTO struct {
	Type    string  `json:"type"`
	ImageID *string `json:"imageId"`
	FileID  *string `json:"fileId"`
}

type imageDTO struct {
	ID           string `json:"id"`
	Extension    string `json:"extension"`
	OriginalURL  string `json:"originalUrl"`
	ThumbnailURL string `json:"thumbnailUrl"`
}

type fileDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Extension string `json:"extension"`
	URL       string `json:"url"`
}

// postInfoDTO 匹配 post.info 的真实 body.post 包装；body 直接携带 post 字段的
// 旧形状不被接受。
type postInfoDTO struct {
	Body struct {
		Post postDTO `json:"post"`
	} `json:"body"`
}

// pageDTO 兼容 home.* 的 body.items 与 post.list* 的 body.posts，均带 nextUrl。
type pageDTO struct {
	Posts    []postDTO `json:"posts"`
	Items    []postDTO `json:"items"`
	NextURL  string    `json:"nextUrl"`
	PageURLs []string  `json:"pageUrls"`
}

type creatorPageDTO struct {
	Plans    *[]creatorDTO `json:"plans"`
	Creators *[]creatorDTO `json:"creators"`
	NextURL  string        `json:"nextUrl"`
	PageURLs []string      `json:"pageUrls"`
}

// fanboxPostListPageLimit is the explicit upstream request parameter captured
// in the repository's FANBOX route evidence for post list operations. It is
// not a local pagination or result cap; continuation remains controlled by
// the server-provided URL.
const fanboxPostListPageLimit = 10

// CurrentUser 使用首页 metadata 验证 Cookie，并返回自动识别出的安全身份摘要。
func (s *Session) CurrentUser(ctx context.Context) (Identity, error) {
	response, err := s.do(ctx, webBaseURL, requestKindFanbox, true, "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	if err != nil {
		return Identity{}, err
	}
	document, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil {
		readFailure := safeExternalError(ctx, "read FANBOX identity page failed", readErr)
		if closeErr != nil {
			return Identity{}, errors.Join(readFailure, safeExternalError(ctx, "close FANBOX identity page failed", closeErr))
		}
		return Identity{}, readFailure
	}
	if closeErr != nil {
		return Identity{}, safeExternalError(ctx, "close FANBOX identity page failed", closeErr)
	}
	return ParseIdentityMetadataHTML(document)
}

// Creator 读取 creator.get 的公开档案，供上层构造稳定且可读的下载目录。
func (s *Session) Creator(ctx context.Context, creatorID string) (CreatorProfile, error) {
	creatorID = strings.TrimSpace(creatorID)
	if creatorID == "" {
		return CreatorProfile{}, errors.New("FANBOX creator id is required")
	}
	endpoint := apiBaseURL + "creator.get?" + url.Values{"creatorId": {creatorID}}.Encode()
	var response struct {
		Body creatorProfileDTO `json:"body"`
	}
	if err := s.getJSON(ctx, endpoint, &response); err != nil {
		return CreatorProfile{}, err
	}
	id := strings.TrimSpace(response.Body.CreatorID)
	if id == "" {
		id = creatorID
	}
	displayName := strings.TrimSpace(response.Body.User.Name)
	// 防御性契约：只要求 creatorId 和非空 display name，其余公开字段尽力填充。
	if id == "" || displayName == "" {
		return CreatorProfile{}, errors.New("FANBOX creator profile is missing creator id or display name")
	}
	return CreatorProfile{
		ID:                id,
		DisplayName:       displayName,
		IconURL:           strings.TrimSpace(response.Body.User.IconURL),
		HasAdultContent:   response.Body.HasAdultContent,
		IsFollowing:       response.Body.IsFollowing,
		CoverURL:          strings.TrimSpace(response.Body.CoverImageURL),
		PlanFee:           response.Body.Plan.Fee,
		HasSupportingPlan: response.Body.Plan.HasSupportingPlan,
	}, nil
}

// Creators 读取 supporting 或 following creator，不执行写操作。nextURL 非空时直接
// 读取上游返回的绝对 continuation URL，不重新拼接或猜测分页参数。
func (s *Session) Creators(ctx context.Context, kind CreatorListKind, nextURL string) (CreatorPage, error) {
	endpoint := strings.TrimSpace(nextURL)
	if endpoint == "" {
		switch kind {
		case CreatorListSupporting:
			endpoint = apiBaseURL + "plan.listSupporting"
		case CreatorListFollowing:
			endpoint = apiBaseURL + "creator.listFollowing"
		default:
			return CreatorPage{}, errors.New("FANBOX creator list kind is invalid")
		}
	}
	var response struct {
		Body json.RawMessage `json:"body"`
	}
	if err := s.getJSON(ctx, endpoint, &response); err != nil {
		return CreatorPage{}, err
	}
	return decodeCreatorPage(response.Body, kind)
}

// CreatorPosts 读取 creator 的 post.listCreator 分页。nextURL 非空时直接取该
// 绝对地址（已通过 fanbox allowlist 校验），而不是把它附加为 query 参数。
func (s *Session) CreatorPosts(ctx context.Context, creatorID, nextURL string) (PostPage, error) {
	if strings.TrimSpace(creatorID) == "" {
		return PostPage{}, errors.New("FANBOX creator id is required")
	}
	endpoint := apiBaseURL + "post.listCreator?" + url.Values{
		"creatorId": {creatorID},
		"limit":     {strconv.Itoa(fanboxPostListPageLimit)},
	}.Encode()
	return s.fetchPostPage(ctx, endpoint, nextURL)
}

// TaggedPosts 读取 creator 的 post.listTagged 分页，语义同 CreatorPosts。
func (s *Session) TaggedPosts(ctx context.Context, creatorID, tag, nextURL string) (PostPage, error) {
	if strings.TrimSpace(creatorID) == "" {
		return PostPage{}, errors.New("FANBOX creator id is required")
	}
	if strings.TrimSpace(tag) == "" {
		return PostPage{}, errors.New("FANBOX tag is required")
	}
	endpoint := apiBaseURL + "post.listTagged?" + url.Values{"creatorId": {creatorID}, "tag": {tag}}.Encode()
	return s.fetchPostPage(ctx, endpoint, nextURL)
}

// Home 读取首页动态流 post.listHome 分页，语义同 CreatorPosts。
func (s *Session) Home(ctx context.Context, nextURL string) (PostPage, error) {
	endpoint := apiBaseURL + "post.listHome?" + url.Values{
		"limit": {strconv.Itoa(fanboxPostListPageLimit)},
	}.Encode()
	return s.fetchHomePage(ctx, endpoint, nextURL)
}

// Supporting 读取支持中的创作者动态 post.listSupporting 分页，语义同 CreatorPosts。
func (s *Session) Supporting(ctx context.Context, nextURL string) (PostPage, error) {
	endpoint := apiBaseURL + "post.listSupporting?" + url.Values{
		"limit": {strconv.Itoa(fanboxPostListPageLimit)},
	}.Encode()
	return s.fetchHomePage(ctx, endpoint, nextURL)
}

func (s *Session) fetchPostPage(ctx context.Context, endpoint, nextURL string) (PostPage, error) {
	return s.fetchPage(ctx, endpoint, nextURL, false)
}

func (s *Session) fetchHomePage(ctx context.Context, endpoint, nextURL string) (PostPage, error) {
	return s.fetchPage(ctx, endpoint, nextURL, true)
}

func (s *Session) fetchPage(ctx context.Context, endpoint, nextURL string, acceptItems bool) (PostPage, error) {
	target := endpoint
	if strings.TrimSpace(nextURL) != "" {
		target = nextURL
	}
	// 服务端返回的 nextUrl 在请求前必须通过 fanbox allowlist，避免把受污染 URL 交给 transport。
	if _, err := parseAllowedURL(target, requestKindFanbox); err != nil {
		return PostPage{}, err
	}
	var response struct {
		Body pageDTO `json:"body"`
	}
	if err := s.getJSON(ctx, target, &response); err != nil {
		return PostPage{}, err
	}
	items := response.Body.Posts
	if acceptItems && items == nil {
		items = response.Body.Items
	}
	posts := make([]Post, 0, len(items))
	for _, item := range items {
		post, err := convertPost(item)
		if err != nil {
			return PostPage{}, err
		}
		posts = append(posts, post)
	}
	return PostPage{Posts: posts, NextURL: firstPageURL(response.Body)}, nil
}

func firstPageURL(page pageDTO) string {
	if len(page.PageURLs) > 0 {
		return page.PageURLs[0]
	}
	return page.NextURL
}

// Post 读取 post.info 的单个 post 详情。
func (s *Session) Post(ctx context.Context, postID string) (Post, error) {
	if strings.TrimSpace(postID) == "" {
		return Post{}, errors.New("FANBOX post id is required")
	}
	endpoint := apiBaseURL + "post.info?" + url.Values{"postId": {postID}}.Encode()
	var response postInfoDTO
	if err := s.getJSON(ctx, endpoint, &response); err != nil {
		return Post{}, err
	}
	return convertPost(response.Body.Post)
}

// OpenMedia 打开经 allowlist 验证的媒体；只有 downloads.fanbox.cc 首跳可携带
// FANBOX Cookie，Pixiv/CDN 与所有 redirect host 都不携带认证信息。
func (s *Session) OpenMedia(ctx context.Context, mediaURL string) (*http.Response, error) {
	return s.OpenMediaWithRequest(ctx, mediaURL, MediaRequest{Method: http.MethodGet})
}

// MediaRequest is the controlled request surface used by the public SDK
// resource adapter. It deliberately has no arbitrary header or Cookie field.
type MediaRequest struct {
	Method          string
	Range           string
	IfNoneMatch     string
	IfModifiedSince string
	IfRange         string
}

// OpenMediaWithRequest opens one media URL with the public resource method and
// the small conditional-request header allowlist. It never accepts a caller
// supplied Cookie or arbitrary header map.
func (s *Session) OpenMediaWithRequest(ctx context.Context, mediaURL string, request MediaRequest) (*http.Response, error) {
	header := make(http.Header)
	for _, field := range []struct{ name, value string }{
		{"Range", request.Range},
		{"If-None-Match", request.IfNoneMatch},
		{"If-Modified-Since", request.IfModifiedSince},
		{"If-Range", request.IfRange},
	} {
		if field.value != "" {
			header.Set(field.name, field.value)
		}
	}
	response, err := s.doWithRequest(ctx, mediaURL, requestKindMedia, true, "*/*", request.Method, header)
	if err != nil {
		return nil, err
	}
	if response.Body == nil {
		return nil, errors.New("FANBOX media response has no body")
	}
	response.Body = &safeMediaReadCloser{ctx: ctx, body: response.Body}
	return response, nil
}

func (s *Session) getJSON(ctx context.Context, endpoint string, target any) error {
	response, err := s.do(ctx, endpoint, requestKindFanbox, true, "application/json, text/plain, */*")
	if err != nil {
		return err
	}
	if response.Body == nil {
		return errors.New("FANBOX API response has no body")
	}
	decodeErr := json.NewDecoder(response.Body).Decode(target)
	closeErr := response.Body.Close()
	if decodeErr != nil {
		decodeFailure := safeExternalError(ctx, "decode FANBOX API response", decodeErr)
		if closeErr != nil {
			return errors.Join(decodeFailure, safeExternalError(ctx, "close FANBOX API response failed", closeErr))
		}
		return decodeFailure
	}
	if closeErr != nil {
		return safeExternalError(ctx, "close FANBOX API response failed", closeErr)
	}
	return nil
}

// safeMediaReadCloser 将外部 media stream 的读写失败映射为无敏感信息的错误；媒体
// 内容仍逐字节透传，调用方可以正常流式读取和显式关闭。
type safeMediaReadCloser struct {
	ctx  context.Context
	body io.ReadCloser
}

func (r *safeMediaReadCloser) Read(body []byte) (int, error) {
	count, err := r.body.Read(body)
	if err == nil || err == io.EOF {
		return count, err
	}
	return count, safeExternalError(r.ctx, "read FANBOX media failed", err)
}

func (r *safeMediaReadCloser) Close() error {
	if err := r.body.Close(); err != nil {
		return safeExternalError(r.ctx, "close FANBOX media failed", err)
	}
	return nil
}

// decodeCreatorPage 兼容两种真实 creator-list envelope：supporting 使用
// body.plans，following 使用 body.creators；直接数组仍被接受。未知 object 不得
// 被静默当作空列表，pageUrls/nextUrl 只作为上游 continuation 原样交给游标层。
func decodeCreatorPage(raw json.RawMessage, kind CreatorListKind) (CreatorPage, error) {
	var direct []creatorDTO
	if err := json.Unmarshal(raw, &direct); err == nil {
		creators, err := creatorListFromDTO(direct)
		return CreatorPage{Creators: creators}, err
	}
	var wrapped creatorPageDTO
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return CreatorPage{}, fmt.Errorf("decode FANBOX %s creators", kind)
	}
	var items *[]creatorDTO
	switch kind {
	case CreatorListSupporting:
		items = wrapped.Plans
	case CreatorListFollowing:
		items = wrapped.Creators
	default:
		return CreatorPage{}, errors.New("FANBOX creator list kind is invalid")
	}
	if items == nil {
		return CreatorPage{}, fmt.Errorf("decode FANBOX %s creators", kind)
	}
	creators, err := creatorListFromDTO(*items)
	if err != nil {
		return CreatorPage{}, err
	}
	return CreatorPage{Creators: creators, NextURL: firstCreatorPageURL(wrapped)}, nil
}

func firstCreatorPageURL(page creatorPageDTO) string {
	if len(page.PageURLs) > 0 {
		return page.PageURLs[0]
	}
	return page.NextURL
}

func creatorListFromDTO(items []creatorDTO) ([]Creator, error) {
	creators := make([]Creator, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.CreatorID)
		if id == "" {
			return nil, errors.New("FANBOX creator response includes an empty creator id")
		}
		creators = append(creators, Creator{ID: id})
	}
	return creators, nil
}

func convertPost(source postDTO) (Post, error) {
	if strings.TrimSpace(source.ID) == "" {
		return Post{}, errors.New("FANBOX post has no id")
	}
	post := Post{
		ID:                source.ID,
		Title:             source.Title,
		PublishedDateTime: source.PublishedDateTime,
		CreatorID:         source.CreatorID,
		FeeRequired:       source.FeeRequired,
		IsRestricted:      source.IsRestricted,
		IsPinned:          source.IsPinned,
		RestrictedFor:     source.RestrictedFor,
		CommentCount:      source.CommentCount,
	}
	if source.Body == nil {
		return post, nil
	}
	body, err := convertPostBody(*source.Body)
	if err != nil {
		return Post{}, err
	}
	post.Body = &body
	return post, nil
}

func convertPostBody(source postBodyDTO) (PostBody, error) {
	body := PostBody{Text: source.Text}
	switch {
	case source.Images != nil:
		body.Images = make([]Image, 0, len(*source.Images))
		body.Assets = make([]Asset, 0, len(*source.Images))
		for _, sourceImage := range *source.Images {
			image, asset, err := convertImage(sourceImage)
			if err != nil {
				return PostBody{}, err
			}
			body.Images = append(body.Images, image)
			body.Assets = append(body.Assets, asset)
		}
	case source.Files != nil:
		body.Files = make([]File, 0, len(*source.Files))
		body.Assets = make([]Asset, 0, len(*source.Files))
		for _, sourceFile := range *source.Files {
			file, asset, err := convertFile(sourceFile)
			if err != nil {
				return PostBody{}, err
			}
			body.Files = append(body.Files, file)
			body.Assets = append(body.Assets, asset)
		}
	case source.Blocks != nil:
		body.Blocks = make([]Block, 0, len(*source.Blocks))
		body.Assets = make([]Asset, 0, len(*source.Blocks))
		for _, sourceBlock := range *source.Blocks {
			block := Block{Type: sourceBlock.Type}
			if sourceBlock.ImageID != nil {
				block.ImageID = *sourceBlock.ImageID
				image, found := source.ImageMap[block.ImageID]
				if !found {
					return PostBody{}, errors.New("FANBOX blog block references a missing image")
				}
				_, asset, err := convertImage(image)
				if err != nil {
					return PostBody{}, err
				}
				body.Assets = append(body.Assets, asset)
			}
			if sourceBlock.FileID != nil {
				block.FileID = *sourceBlock.FileID
				file, found := source.FileMap[block.FileID]
				if !found {
					return PostBody{}, errors.New("FANBOX blog block references a missing file")
				}
				_, asset, err := convertFile(file)
				if err != nil {
					return PostBody{}, err
				}
				body.Assets = append(body.Assets, asset)
			}
			body.Blocks = append(body.Blocks, block)
		}
	}
	return body, nil
}

func convertImage(source imageDTO) (Image, Asset, error) {
	if strings.TrimSpace(source.ID) == "" {
		return Image{}, Asset{}, errors.New("FANBOX image has no id")
	}
	if _, err := parseAllowedURL(source.OriginalURL, requestKindMedia); err != nil {
		return Image{}, Asset{}, err
	}
	if source.ThumbnailURL != "" {
		if _, err := parseAllowedURL(source.ThumbnailURL, requestKindMedia); err != nil {
			return Image{}, Asset{}, err
		}
	}
	image := Image{ID: source.ID, Extension: source.Extension, OriginalURL: source.OriginalURL, ThumbnailURL: source.ThumbnailURL}
	return image, Asset{ID: source.ID, Kind: AssetKindImage, Extension: source.Extension, URL: source.OriginalURL, ThumbnailURL: source.ThumbnailURL}, nil
}

func convertFile(source fileDTO) (File, Asset, error) {
	if strings.TrimSpace(source.ID) == "" {
		return File{}, Asset{}, errors.New("FANBOX file has no id")
	}
	if _, err := parseAllowedURL(source.URL, requestKindMedia); err != nil {
		return File{}, Asset{}, err
	}
	file := File{ID: source.ID, Name: source.Name, Extension: source.Extension, URL: source.URL}
	return file, Asset{ID: source.ID, Kind: AssetKindFile, Name: source.Name, Extension: source.Extension, URL: source.URL}, nil
}
