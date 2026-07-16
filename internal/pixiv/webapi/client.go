package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/model"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/protocol"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/text"
)

const (
	DefaultWebBase = protocol.WebAPIBase
	defaultUA      = protocol.WebUserAgent
	// 两个值是对应 Pixiv Web endpoint 的固定 wire batch 边界；边界测试
	// 分别以 59/60 与 49/50 锁定换页行为，不是本地结果条数限制。
	artworkSearchPageSize = 60
	illustRankingPageSize = 50
)

// ErrMalformedResponse 标识成功 HTTP 响应无法构成约定 JSON，不包含原始响应体。
var ErrMalformedResponse = protocol.ErrMalformedResponse

// ErrUnrepresentablePagination 标识 cursor offset 无法安全换算为 Web 页码和下一页边界。
var ErrUnrepresentablePagination = errors.New("web api pagination cannot represent cursor offset")

// EnvelopeError 保留内部兼容名称；message 不得越过 Web adapter。
type EnvelopeError = protocol.Failure

// IllustPagesError 保留匿名详情流程中 pages 子阶段，供 facade 精确标注 operation。
type IllustPagesError struct {
	err error
}

func (e *IllustPagesError) Error() string { return fmt.Sprintf("web illust pages: %v", e.err) }
func (e *IllustPagesError) Unwrap() error { return e.err }

type Client struct {
	httpClient *http.Client
	webBase    string
	userAgent  string
}

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

func WithWebBase(base string) Option {
	return func(c *Client) {
		if base != "" {
			c.webBase = strings.TrimRight(base, "/")
		}
	}
}

func New(opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{},
		webBase:    DefaultWebBase,
		userAgent:  defaultUA,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) SearchIllust(ctx context.Context, word, target, sort, duration string, offset int) (*model.IllustList, error) {
	pagination, err := checkedWebPagination(offset, artworkSearchPageSize)
	if err != nil {
		return nil, err
	}
	q := url.Values{
		"word":   {word},
		"order":  {webSearchOrder(sort)},
		"mode":   {"all"},
		"p":      {strconv.Itoa(pagination.page)},
		"s_mode": {webSearchMode(target)},
		"type":   {"all"},
		"lang":   {"zh"},
	}
	if err := setDuration(q, duration); err != nil {
		return nil, err
	}
	var out ajaxEnvelope[webSearchBody]
	if err := c.getJSON(ctx, protocol.WebSearchArtworks(word), q, &out); err != nil {
		return nil, err
	}
	if out.Error {
		return nil, webEnvelopeError(out.Message)
	}
	group := out.Body.IllustManga
	if (!group.Data.Valid || len(group.Data.Items) == 0) && out.Body.Illust.Data.Valid && len(out.Body.Illust.Data.Items) > 0 {
		group = out.Body.Illust
	} else if !group.Data.Valid && out.Body.Illust.Data.Valid {
		group = out.Body.Illust
	}
	if !group.Data.Present || !group.Data.Valid {
		return nil, ErrMalformedResponse
	}
	items := group.Data.Items
	rawCount := len(items)
	items = trimWebPageOffset(items, offset, artworkSearchPageSize)
	illusts := make([]model.Illust, 0, len(items))
	for _, item := range items {
		if item.ID <= 0 {
			return nil, ErrMalformedResponse
		}
		illusts = append(illusts, mapSearchIllust(item))
	}
	result := &model.IllustList{Illusts: illusts}
	if webHasNext(offset, rawCount, int64(group.Total), artworkSearchPageSize) {
		result.NextOffset = pagination.nextOffset
		result.ContinuationExists = true
	}
	return result, nil
}

func (c *Client) IllustDetail(ctx context.Context, id int64) (*model.IllustDetail, error) {
	var detail ajaxEnvelope[webIllustDetail]
	if err := c.getJSON(ctx, protocol.WebIllustDetail(id), url.Values{"lang": {"zh"}}, &detail); err != nil {
		return nil, err
	}
	if detail.Error {
		return nil, webEnvelopeError(detail.Message)
	}
	if !detail.bodyPresent || int64(firstFlexInt64(detail.Body.ID, detail.Body.IllustID)) <= 0 {
		return nil, ErrMalformedResponse
	}
	pages, err := c.IllustPages(ctx, id)
	if err != nil {
		return nil, &IllustPagesError{err: err}
	}
	illust := mapDetailIllust(detail.Body, pages)
	return &model.IllustDetail{Illust: illust}, nil
}

func (c *Client) IllustPages(ctx context.Context, id int64) ([]model.MetaPage, error) {
	var out ajaxEnvelope[[]webPage]
	if err := c.getJSON(ctx, protocol.WebIllustPages(id), url.Values{"lang": {"zh"}}, &out); err != nil {
		return nil, err
	}
	if out.Error {
		return nil, webEnvelopeError(out.Message)
	}
	if !out.bodyPresent {
		return nil, ErrMalformedResponse
	}
	pages := make([]model.MetaPage, 0, len(out.Body))
	for index, page := range out.Body {
		pages = append(pages, model.MetaPage{
			PageIndex: index,
			Width:     int(page.Width),
			Height:    int(page.Height),
			Extension: imageExtension(page.URLs.Original),
			ImageURLs: mapPageURLs(page.URLs),
		})
	}
	return pages, nil
}

func (c *Client) IllustRanking(ctx context.Context, mode, date string, offset int) (*model.IllustList, error) {
	pagination, err := checkedWebPagination(offset, illustRankingPageSize)
	if err != nil {
		return nil, err
	}
	q := url.Values{
		"format": {"json"},
		"mode":   {webRankingMode(mode)},
		"p":      {strconv.Itoa(pagination.page)},
	}
	if date != "" {
		q.Set("date", strings.ReplaceAll(date, "-", ""))
	}
	var out webRankingResponse
	if err := c.getJSON(ctx, protocol.WebRanking, q, &out); err != nil {
		return nil, err
	}
	if !out.Contents.Present || !out.Contents.Valid {
		return nil, ErrMalformedResponse
	}
	for _, item := range out.Contents.Items {
		if item.IllustID <= 0 {
			return nil, ErrMalformedResponse
		}
	}
	rawCount := len(out.Contents.Items)
	items := trimWebPageOffset(out.Contents.Items, offset, illustRankingPageSize)
	illusts := make([]model.Illust, 0, len(items))
	for _, item := range items {
		illusts = append(illusts, mapRankingIllust(item))
	}
	result := &model.IllustList{Illusts: illusts}
	if webHasNext(offset, rawCount, int64(out.RankTotal), illustRankingPageSize) {
		result.NextOffset = pagination.nextOffset
		result.ContinuationExists = true
	}
	return result, nil
}

func (c *Client) SearchUser(ctx context.Context, word string, offset int) (*model.UserPreviewList, error) {
	illusts, err := c.SearchIllust(ctx, word, string(model.SearchTargetPartialMatchForTags), string(model.SortModeDateDesc), "", offset)
	if err != nil {
		return nil, err
	}
	seen := make(map[int64]struct{})
	users := make([]model.UserPreview, 0, len(illusts.Illusts))
	for _, illust := range illusts.Illusts {
		if illust.User.ID == 0 {
			return nil, ErrMalformedResponse
		}
		if _, ok := seen[illust.User.ID]; ok {
			continue
		}
		seen[illust.User.ID] = struct{}{}
		users = append(users, model.UserPreview{User: illust.User})
	}
	return &model.UserPreviewList{
		UserPreviews:       users,
		NextOffset:         illusts.NextOffset,
		ContinuationExists: illusts.ContinuationExists,
	}, nil
}

func (c *Client) UgoiraMetadata(ctx context.Context, id int64) (*model.UgoiraMetadataResult, error) {
	var out ajaxEnvelope[webUgoiraMeta]
	if err := c.getJSON(ctx, protocol.WebUgoiraMetadata(id), url.Values{"lang": {"zh"}}, &out); err != nil {
		return nil, err
	}
	if out.Error {
		return nil, webEnvelopeError(out.Message)
	}
	if !out.bodyPresent || out.Body.OriginalSrc == "" || len(out.Body.Frames) == 0 {
		return nil, ErrMalformedResponse
	}
	var result model.UgoiraMetadataResult
	result.UgoiraMetadata.ZipURLs.Medium = out.Body.Src
	result.UgoiraMetadata.ZipURLs.Original = out.Body.OriginalSrc
	result.UgoiraMetadata.Frames = make([]model.UgoiraFrame, 0, len(out.Body.Frames))
	for _, frame := range out.Body.Frames {
		if frame.File == "" {
			return nil, ErrMalformedResponse
		}
		result.UgoiraMetadata.Frames = append(result.UgoiraMetadata.Frames, model.UgoiraFrame{
			File:  frame.File,
			Delay: int(frame.Delay),
		})
	}
	return &result, nil
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	rawURL, err := c.webURL(path, query)
	if err != nil {
		return protocol.Transport(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return protocol.Transport(err)
	}
	c.setHeaders(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return protocol.Transport(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		// reader 失败可能来自底层 transport；不能让其 URL 或代理诊断跨越
		// Web adapter，统一转换为 protocol 的脱敏失败。
		return protocol.Transport(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return protocol.HTTPStatus(resp.StatusCode)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return protocol.MalformedResponse()
	}
	if err := json.Unmarshal(body, out); err != nil {
		return protocol.MalformedResponse()
	}
	return nil
}

func (c *Client) webURL(path string, query url.Values) (string, error) {
	base, err := url.Parse(c.webBase)
	if err != nil {
		return "", err
	}
	rel, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	resolved := base.ResolveReference(rel)
	if len(query) > 0 {
		resolved.RawQuery = query.Encode()
	}
	return resolved.String(), nil
}

func (c *Client) setHeaders(req *http.Request) {
	for key, value := range protocol.WebHeaders() {
		if key == "User-Agent" && c.userAgent != "" {
			value = c.userAgent
		}
		req.Header.Set(key, value)
	}
}

type APIError = protocol.Failure

func webEnvelopeError(string) error {
	return protocol.UpstreamRejected()
}

type webPagination struct {
	page       int
	nextOffset int
}

func checkedWebPagination(offset, pageSize int) (webPagination, error) {
	if offset < 0 || pageSize <= 0 {
		return webPagination{}, ErrUnrepresentablePagination
	}
	quotient := offset / pageSize
	maxInt := int(^uint(0) >> 1)
	if quotient == maxInt {
		return webPagination{}, ErrUnrepresentablePagination
	}
	page := quotient + 1
	if page > maxInt/pageSize {
		return webPagination{}, ErrUnrepresentablePagination
	}
	// Pixiv Web 使用固定批次页码；同时预检下一页边界，避免响应后产生负 cursor。
	return webPagination{page: page, nextOffset: page * pageSize}, nil
}

func trimWebPageOffset[T any](items []T, offset, pageSize int) []T {
	if offset <= 0 || pageSize <= 0 {
		return items
	}
	skip := offset % pageSize
	if skip >= len(items) {
		return nil
	}
	return items[skip:]
}

func webHasNext(offset, rawCount int, total int64, pageSize int) bool {
	if rawCount == 0 {
		return false
	}
	if total > 0 {
		pageStart := int64(offset - offset%pageSize)
		if pageStart >= total {
			return false
		}
		// 用减法表达 pageStart+rawCount<total，避免靠近 MaxInt 时加法回绕。
		return int64(rawCount) < total-pageStart
	}
	// 无 total 时，只在完整上游批次后继续；下一空批次会自然收敛。
	return rawCount >= pageSize
}

func webSearchOrder(sort string) string {
	switch sort {
	case "", string(model.SortModeDateDesc):
		return "date_d"
	case string(model.SortModeDateAsc):
		return "date"
	default:
		return sort
	}
}

func webSearchMode(target string) string {
	switch target {
	case "", string(model.SearchTargetPartialMatchForTags):
		return "s_tag"
	case string(model.SearchTargetExactMatchForTags):
		return "s_tag_full"
	case string(model.SearchTargetTitleAndCaption):
		return "s_tc"
	default:
		return target
	}
}

func webRankingMode(mode string) string {
	switch mode {
	case "", string(model.RankingModeDay):
		return "daily"
	case string(model.RankingModeDayMale):
		return "male"
	case string(model.RankingModeDayFemale):
		return "female"
	case string(model.RankingModeWeek):
		return "weekly"
	case string(model.RankingModeWeekOriginal):
		return "original"
	case string(model.RankingModeWeekRookie):
		return "rookie"
	case string(model.RankingModeMonth):
		return "monthly"
	default:
		return mode
	}
}

func setDuration(q url.Values, duration string) error {
	if duration == "" {
		return nil
	}
	now := time.Now()
	var start time.Time
	switch duration {
	case "within_last_day":
		start = now.AddDate(0, 0, -1)
	case "within_last_week":
		start = now.AddDate(0, 0, -7)
	case "within_last_month":
		start = now.AddDate(0, -1, 0)
	default:
		return fmt.Errorf("pixiv web fallback does not support duration %q", duration)
	}
	q.Set("scd", start.Format("2006-01-02"))
	q.Set("ecd", now.Format("2006-01-02"))
	return nil
}

func mapSearchIllust(item webSearchIllust) model.Illust {
	tags := make([]model.Tag, 0, len(item.Tags))
	for _, tag := range item.Tags {
		tags = append(tags, model.Tag{Name: tag})
	}
	imageURLs := model.ImageURLs{SquareMedium: item.URL, Medium: item.URL}
	return model.Illust{
		ID:        int64(item.ID),
		Title:     item.Title,
		Type:      illustType(int(item.IllustType)),
		PageCount: int(item.PageCount),
		XRestrict: int(item.XRestrict),
		User: model.User{
			ID:      int64(item.UserID),
			Name:    item.UserName,
			Account: strconv.FormatInt(int64(item.UserID), 10),
		},
		Tags:      tags,
		ImageURLs: imageURLs,
		AIType:    int(item.AIType),
	}
}

func mapDetailIllust(item webIllustDetail, metaPages []model.MetaPage) model.Illust {
	tags := mapDetailTags(item.Tags.Tags)
	imageURLs := mapDetailURLs(item.URLs)
	if len(metaPages) > 0 {
		imageURLs = firstImageURLs(imageURLs, metaPages[0].ImageURLs)
	}
	pageCount := int(item.PageCount)
	if pageCount == 0 && len(metaPages) > 0 {
		pageCount = len(metaPages)
	}
	illust := model.Illust{
		ID:             int64(firstFlexInt64(item.ID, item.IllustID)),
		Title:          text.FirstNonEmpty(item.Title, item.IllustTitle),
		Type:           illustType(int(item.IllustType)),
		PageCount:      pageCount,
		TotalBookmarks: int(item.BookmarkCount),
		TotalView:      int(item.ViewCount),
		XRestrict:      int(item.XRestrict),
		User: model.User{
			ID:      int64(item.UserID),
			Name:    item.UserName,
			Account: strconv.FormatInt(int64(item.UserID), 10),
		},
		Tags:       tags,
		ImageURLs:  imageURLs,
		MetaPages:  metaPages,
		AIType:     int(item.AIType),
		CreateDate: item.CreateDate,
		Width:      int(item.Width),
		Height:     int(item.Height),
	}
	if len(metaPages) == 1 {
		illust.MetaSinglePage.OriginalImageURL = metaPages[0].ImageURLs.Original
	}
	return illust
}

func imageExtension(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(path.Ext(parsed.Path), ".")
}

func mapRankingIllust(item webRankingItem) model.Illust {
	tags := make([]model.Tag, 0, len(item.Tags))
	for _, tag := range item.Tags {
		tags = append(tags, model.Tag{Name: tag})
	}
	return model.Illust{
		ID:             int64(item.IllustID),
		Title:          item.Title,
		Type:           illustType(int(item.IllustType)),
		PageCount:      int(item.IllustPageCount),
		TotalBookmarks: int(item.RatingCount),
		TotalView:      int(item.ViewCount),
		User: model.User{
			ID:      int64(item.UserID),
			Name:    item.UserName,
			Account: strconv.FormatInt(int64(item.UserID), 10),
		},
		Tags: tags,
		ImageURLs: model.ImageURLs{
			SquareMedium: item.URL,
			Medium:       item.URL,
			Large:        item.URL,
		},
	}
}

func mapDetailTags(items []webTag) []model.Tag {
	tags := make([]model.Tag, 0, len(items))
	for _, item := range items {
		tags = append(tags, model.Tag{Name: item.Tag, TranslatedName: item.Translation.En})
	}
	return tags
}

func mapDetailURLs(urls webDetailURLs) model.ImageURLs {
	return model.ImageURLs{
		SquareMedium: text.FirstNonEmpty(urls.Mini, urls.ThumbMini, urls.Small),
		Medium:       text.FirstNonEmpty(urls.Small, urls.Regular),
		Large:        text.FirstNonEmpty(urls.Regular, urls.Original),
		Original:     urls.Original,
	}
}

func mapPageURLs(urls webPageURLs) model.ImageURLs {
	return model.ImageURLs{
		SquareMedium: urls.ThumbMini,
		Medium:       text.FirstNonEmpty(urls.Small, urls.Regular),
		Large:        text.FirstNonEmpty(urls.Regular, urls.Original),
		Original:     urls.Original,
	}
}

func firstImageURLs(primary, fallback model.ImageURLs) model.ImageURLs {
	return model.ImageURLs{
		SquareMedium: text.FirstNonEmpty(primary.SquareMedium, fallback.SquareMedium),
		Medium:       text.FirstNonEmpty(primary.Medium, fallback.Medium),
		Large:        text.FirstNonEmpty(primary.Large, fallback.Large),
		Original:     text.FirstNonEmpty(primary.Original, fallback.Original),
	}
}

func illustType(value int) string {
	switch value {
	case 1:
		return string(model.IllustTypeManga)
	case 2:
		return string(model.IllustTypeUgoira)
	default:
		return string(model.IllustTypeIllust)
	}
}

func firstFlexInt64(values ...flexInt64) flexInt64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

type ajaxEnvelope[T any] struct {
	Error   bool   `json:"error"`
	Message string `json:"message"`
	Body    T      `json:"body"`

	bodyPresent bool
}

func (e *ajaxEnvelope[T]) UnmarshalJSON(data []byte) error {
	var wire struct {
		Error   bool            `json:"error"`
		Message string          `json:"message"`
		Body    json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	e.Error = wire.Error
	e.Message = wire.Message
	body := bytes.TrimSpace(wire.Body)
	if len(body) == 0 || bytes.Equal(body, []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(body, &e.Body); err != nil {
		return err
	}
	e.bodyPresent = true
	return nil
}

type webSearchBody struct {
	IllustManga webSearchGroup `json:"illustManga"`
	Illust      webSearchGroup `json:"illust"`
}

type webSearchGroup struct {
	Data  requiredWebList[webSearchIllust] `json:"data"`
	Total flexInt64                        `json:"total"`
}

type requiredWebList[T any] struct {
	Items   []T
	Present bool
	Valid   bool
}

func (l *requiredWebList[T]) UnmarshalJSON(data []byte) error {
	*l = requiredWebList[T]{}
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

type webSearchIllust struct {
	ID            flexInt64 `json:"id"`
	Title         string    `json:"title"`
	IllustType    flexInt   `json:"illustType"`
	XRestrict     flexInt   `json:"xRestrict"`
	AIType        flexInt   `json:"aiType"`
	URL           string    `json:"url"`
	Tags          []string  `json:"tags"`
	UserID        flexInt64 `json:"userId"`
	UserName      string    `json:"userName"`
	PageCount     flexInt   `json:"pageCount"`
	ProfileImgURL string    `json:"profileImageUrl"`
}

type webIllustDetail struct {
	ID            flexInt64     `json:"id"`
	IllustID      flexInt64     `json:"illustId"`
	Title         string        `json:"title"`
	IllustTitle   string        `json:"illustTitle"`
	IllustType    flexInt       `json:"illustType"`
	XRestrict     flexInt       `json:"xRestrict"`
	PageCount     flexInt       `json:"pageCount"`
	UserID        flexInt64     `json:"userId"`
	UserName      string        `json:"userName"`
	BookmarkCount flexInt       `json:"bookmarkCount"`
	ViewCount     flexInt       `json:"viewCount"`
	AIType        flexInt       `json:"aiType"`
	CreateDate    string        `json:"createDate"`
	Width         flexInt       `json:"width"`
	Height        flexInt       `json:"height"`
	Tags          webDetailTags `json:"tags"`
	URLs          webDetailURLs `json:"urls"`
}

type webDetailTags struct {
	Tags []webTag `json:"tags"`
}

type webTag struct {
	Tag         string `json:"tag"`
	Translation struct {
		En string `json:"en"`
	} `json:"translation"`
}

type webDetailURLs struct {
	Mini      string `json:"mini"`
	ThumbMini string `json:"thumb_mini"`
	Small     string `json:"small"`
	Regular   string `json:"regular"`
	Original  string `json:"original"`
}

type webPage struct {
	URLs   webPageURLs `json:"urls"`
	Width  flexInt     `json:"width"`
	Height flexInt     `json:"height"`
}

type webPageURLs struct {
	ThumbMini string `json:"thumb_mini"`
	Small     string `json:"small"`
	Regular   string `json:"regular"`
	Original  string `json:"original"`
}

type webRankingResponse struct {
	Contents  requiredWebList[webRankingItem] `json:"contents"`
	RankTotal flexInt64                       `json:"rank_total"`
}

type webRankingItem struct {
	Title           string    `json:"title"`
	Tags            []string  `json:"tags"`
	URL             string    `json:"url"`
	IllustType      flexInt   `json:"illust_type"`
	IllustPageCount flexInt   `json:"illust_page_count"`
	UserName        string    `json:"user_name"`
	IllustID        flexInt64 `json:"illust_id"`
	UserID          flexInt64 `json:"user_id"`
	RatingCount     flexInt   `json:"rating_count"`
	ViewCount       flexInt   `json:"view_count"`
}

type webUgoiraMeta struct {
	OriginalSrc string           `json:"originalSrc"`
	Src         string           `json:"src"`
	Frames      []webUgoiraFrame `json:"frames"`
}

type webUgoiraFrame struct {
	File  string  `json:"file"`
	Delay flexInt `json:"delay"`
}

type flexInt64 int64

func (v *flexInt64) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) || len(data) == 0 {
		*v = 0
		return nil
	}
	if data[0] == '"' {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		if text == "" {
			*v = 0
			return nil
		}
		parsed, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return err
		}
		*v = flexInt64(parsed)
		return nil
	}
	parsed, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return err
	}
	*v = flexInt64(parsed)
	return nil
}

type flexInt int

func (v *flexInt) UnmarshalJSON(data []byte) error {
	var value flexInt64
	if err := value.UnmarshalJSON(data); err != nil {
		return err
	}
	*v = flexInt(value)
	return nil
}
