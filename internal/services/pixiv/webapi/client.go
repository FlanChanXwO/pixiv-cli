package webapi

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/model"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

const (
	DefaultWebBase = protocol.WebAPIBase
	defaultUA      = protocol.WebUserAgent
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
		httpClient: &http.Client{},
		webBase:    DefaultWebBase,
		userAgent:  defaultUA,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) SearchIllust(ctx context.Context, word, target, sort, duration, startDate, endDate string, offset int, filterOptions ...model.SearchIllustFilters) (*model.IllustList, error) {
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
	filters := model.SearchIllustFilters{}
	if len(filterOptions) > 0 {
		filters = filterOptions[0]
		setWebSearchIllustFilters(q, filters)
	}
	if err := setDuration(q, duration); err != nil {
		return nil, err
	}
	setOptionalSearchValue(q, "scd", startDate)
	setOptionalSearchValue(q, "ecd", endDate)
	var out ajaxEnvelope[webSearchBody]
	if err := c.getJSON(ctx, webSearchIllustPath(word, filters.ContentType, q), q, &out); err != nil {
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
		return nil, protocol.MalformedResponse()
	}
	items := group.Data.Items
	rawCount := len(items)
	items = trimWebPageOffset(items, offset, artworkSearchPageSize)
	illusts := make([]model.Illust, 0, len(items))
	for _, item := range items {
		if item.ID <= 0 {
			return nil, protocol.MalformedResponse()
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

func webSearchIllustPath(word, contentType string, query url.Values) string {
	escapedWord := url.PathEscape(word)
	switch contentType {
	case "manga":
		query.Del("type")
		return "/ajax/search/manga/" + escapedWord
	case "illust", "ugoira":
		query.Set("type", contentType)
		return "/ajax/search/illustrations/" + escapedWord
	case "illust-and-ugoira":
		query.Set("type", "all")
		return "/ajax/search/illustrations/" + escapedWord
	default:
		return protocol.WebSearchArtworks(word)
	}
}

func setWebSearchIllustFilters(query url.Values, filters model.SearchIllustFilters) {
	switch filters.AspectRatio {
	case "landscape":
		query.Set("ratio", "0.5")
	case "portrait":
		query.Set("ratio", "-0.5")
	case "square":
		query.Set("ratio", "0")
	}
	setOptionalSearchValue(query, "tool", filters.Tool)
	switch filters.Resolution {
	case "high":
		query.Set("wlt", "3000")
		query.Set("hlt", "3000")
	case "medium":
		query.Set("wlt", "1000")
		query.Set("wgt", "2999")
		query.Set("hlt", "1000")
		query.Set("hgt", "2999")
	case "low":
		query.Set("wgt", "999")
		query.Set("hgt", "999")
	}
}

func setOptionalSearchValue(query url.Values, key, value string) {
	if value != "" {
		query.Set(key, value)
	}
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
		return nil, protocol.MalformedResponse()
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
		return nil, protocol.MalformedResponse()
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
		return nil, protocol.MalformedResponse()
	}
	for _, item := range out.Contents.Items {
		if item.IllustID <= 0 {
			return nil, protocol.MalformedResponse()
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
	illusts, err := c.SearchIllust(ctx, word, string(model.SearchTargetPartialMatchForTags), string(model.SortModeDateDesc), "", "", "", offset)
	if err != nil {
		return nil, err
	}
	seen := make(map[int64]struct{})
	users := make([]model.UserPreview, 0, len(illusts.Illusts))
	for _, illust := range illusts.Illusts {
		if illust.User.ID == 0 {
			return nil, protocol.MalformedResponse()
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
		return nil, protocol.MalformedResponse()
	}
	var result model.UgoiraMetadataResult
	result.UgoiraMetadata.ZipURLs.Medium = out.Body.Src
	result.UgoiraMetadata.ZipURLs.Original = out.Body.OriginalSrc
	result.UgoiraMetadata.Frames = make([]model.UgoiraFrame, 0, len(out.Body.Frames))
	for _, frame := range out.Body.Frames {
		if frame.File == "" {
			return nil, protocol.MalformedResponse()
		}
		result.UgoiraMetadata.Frames = append(result.UgoiraMetadata.Frames, model.UgoiraFrame{
			File:  frame.File,
			Delay: int(frame.Delay),
		})
	}
	return &result, nil
}
