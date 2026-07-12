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
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/text"
)

const (
	DefaultWebBase = "https://www.pixiv.net"
	defaultUA      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"
)

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
		httpClient: http.DefaultClient,
		webBase:    DefaultWebBase,
		userAgent:  defaultUA,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) SearchIllust(ctx context.Context, word, target, sort, duration string, offset int) (*model.IllustList, error) {
	q := url.Values{
		"word":   {word},
		"order":  {webSearchOrder(sort)},
		"mode":   {"all"},
		"p":      {strconv.Itoa(webPageFromOffset(offset, 60))},
		"s_mode": {webSearchMode(target)},
		"type":   {"all"},
		"lang":   {"zh"},
	}
	if err := setDuration(q, duration); err != nil {
		return nil, err
	}
	var out ajaxEnvelope[webSearchBody]
	if err := c.getJSON(ctx, "/ajax/search/artworks/"+url.PathEscape(word), q, &out); err != nil {
		return nil, err
	}
	if out.Error {
		return nil, webEnvelopeError(out.Message)
	}
	items := out.Body.IllustManga.Data
	if len(items) == 0 {
		items = out.Body.Illust.Data
	}
	items = trimWebPageOffset(items, offset, 60)
	illusts := make([]model.Illust, 0, len(items))
	for _, item := range items {
		illusts = append(illusts, mapSearchIllust(item))
	}
	return &model.IllustList{Illusts: illusts}, nil
}

func (c *Client) IllustDetail(ctx context.Context, id int64) (*model.IllustDetail, error) {
	var detail ajaxEnvelope[webIllustDetail]
	if err := c.getJSON(ctx, fmt.Sprintf("/ajax/illust/%d", id), url.Values{"lang": {"zh"}}, &detail); err != nil {
		return nil, err
	}
	if detail.Error {
		return nil, webEnvelopeError(detail.Message)
	}
	pages, err := c.IllustPages(ctx, id)
	if err != nil {
		return nil, err
	}
	illust := mapDetailIllust(detail.Body, pages)
	return &model.IllustDetail{Illust: illust}, nil
}

func (c *Client) IllustPages(ctx context.Context, id int64) ([]model.MetaPage, error) {
	var out ajaxEnvelope[[]webPage]
	if err := c.getJSON(ctx, fmt.Sprintf("/ajax/illust/%d/pages", id), url.Values{"lang": {"zh"}}, &out); err != nil {
		return nil, err
	}
	if out.Error {
		return nil, webEnvelopeError(out.Message)
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
	q := url.Values{
		"format": {"json"},
		"mode":   {webRankingMode(mode)},
		"p":      {strconv.Itoa(webPageFromOffset(offset, 50))},
	}
	if date != "" {
		q.Set("date", strings.ReplaceAll(date, "-", ""))
	}
	var out webRankingResponse
	if err := c.getJSON(ctx, "/ranking.php", q, &out); err != nil {
		return nil, err
	}
	items := trimWebPageOffset(out.Contents, offset, 50)
	illusts := make([]model.Illust, 0, len(items))
	for _, item := range items {
		illusts = append(illusts, mapRankingIllust(item))
	}
	return &model.IllustList{Illusts: illusts}, nil
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
			continue
		}
		if _, ok := seen[illust.User.ID]; ok {
			continue
		}
		seen[illust.User.ID] = struct{}{}
		users = append(users, model.UserPreview{User: illust.User})
	}
	return &model.UserPreviewList{UserPreviews: users}, nil
}

func (c *Client) UgoiraMetadata(ctx context.Context, id int64) (*model.UgoiraMetadataResult, error) {
	var out ajaxEnvelope[webUgoiraMeta]
	if err := c.getJSON(ctx, fmt.Sprintf("/ajax/illust/%d/ugoira_meta", id), url.Values{"lang": {"zh"}}, &out); err != nil {
		return nil, err
	}
	if out.Error {
		return nil, webEnvelopeError(out.Message)
	}
	var result model.UgoiraMetadataResult
	result.UgoiraMetadata.ZipURLs.Medium = text.FirstNonEmpty(out.Body.OriginalSrc, out.Body.Src)
	result.UgoiraMetadata.Frames = make([]model.UgoiraFrame, 0, len(out.Body.Frames))
	for _, frame := range out.Body.Frames {
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
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	c.setHeaders(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return errors.New("empty response")
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode web response: %w", err)
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
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json,text/plain,*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,ja;q=0.8,en;q=0.7")
}

type APIError struct {
	StatusCode int
	Body       string
}

func (e APIError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("pixiv web api error: status %d", e.StatusCode)
	}
	return fmt.Sprintf("pixiv web api error: status %d: %s", e.StatusCode, e.Body)
}

func webEnvelopeError(message string) error {
	if message == "" {
		return errors.New("pixiv web api error")
	}
	return fmt.Errorf("pixiv web api error: %s", message)
}

func webPageFromOffset(offset, pageSize int) int {
	if offset <= 0 {
		return 1
	}
	// Pixiv web ranking/search 使用页码而非 app API offset；按其固定页容量映射到对应页。
	return offset/pageSize + 1
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
	case string(model.RankingModeWeek):
		return "weekly"
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
		Tags:      tags,
		ImageURLs: imageURLs,
		MetaPages: metaPages,
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
}

type webSearchBody struct {
	IllustManga webSearchGroup `json:"illustManga"`
	Illust      webSearchGroup `json:"illust"`
}

type webSearchGroup struct {
	Data []webSearchIllust `json:"data"`
}

type webSearchIllust struct {
	ID            flexInt64 `json:"id"`
	Title         string    `json:"title"`
	IllustType    flexInt   `json:"illustType"`
	XRestrict     flexInt   `json:"xRestrict"`
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
	Contents []webRankingItem `json:"contents"`
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
