package pixiv

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/model"
)

type illustRelatedQuery struct {
	IllustID int64 `json:"illust_id"`
}

// IllustPages 返回作品的全部页面元数据；该接口不分页。
func (c *Client) IllustPages(ctx context.Context, illustID int64) ([]MetaPage, error) {
	if illustID <= 0 {
		return nil, invalidIllustArgument(OperationIllustPages, errors.New("illust id must be positive"))
	}
	if err := c.requireRoute(OperationIllustPages, routeWeb, illustID, 0); err != nil {
		return nil, err
	}
	pages, err := c.web.IllustPages(ctx, illustID)
	if err != nil {
		return nil, mapWebError(err, OperationIllustPages, illustID)
	}
	return mapMetaPages(pages), nil
}

// IllustRelated 返回与指定作品相关的一个 App API 批次。
func (c *Client) IllustRelated(ctx context.Context, request IllustRelatedRequest) (*IllustListResult, error) {
	if request.IllustID <= 0 {
		return nil, invalidIllustArgument(OperationIllustRelated, errors.New("illust id must be positive"))
	}
	query := illustRelatedQuery{IllustID: request.IllustID}
	digest := queryDigest(OperationIllustRelated, query)
	offset, err := cursorIllustOffset(request.Cursor, OperationIllustRelated, digest, request.IllustID)
	if err != nil {
		return nil, err
	}
	if err := c.requireRoute(OperationIllustRelated, routeApp, request.IllustID, 0); err != nil {
		return nil, err
	}
	list, err := c.app.IllustRelated(ctx, request.IllustID, offset)
	if err != nil {
		return nil, mapAppError(err, OperationIllustRelated, request.IllustID)
	}
	return publicIllustList(list, OperationIllustRelated, digest, "offset"), nil
}

// TrendingTagsIllust 返回 App API 当前的插画趋势标签。
func (c *Client) TrendingTagsIllust(ctx context.Context) (*TrendingTagsIllustResult, error) {
	if err := c.requireRoute(OperationTrendingTagsIllust, routeApp, 0, 0); err != nil {
		return nil, err
	}
	result, err := c.app.TrendingTagsIllust(ctx)
	if err != nil {
		return nil, mapAppError(err, OperationTrendingTagsIllust, 0)
	}
	out := &TrendingTagsIllustResult{TrendTags: make([]TrendTag, len(result.TrendTags))}
	for i, item := range result.TrendTags {
		out.TrendTags[i] = TrendTag{Tag: item.Tag, TranslatedName: item.TranslatedName, Illust: mapIllust(item.Illust)}
	}
	return out, nil
}

// UgoiraMetadata 以 App 数据为主，并用 Web 的真实 originalSrc 补全原始压缩包 URL。
func (c *Client) UgoiraMetadata(ctx context.Context, illustID int64) (*UgoiraMetadataResult, error) {
	if illustID <= 0 {
		return nil, invalidIllustArgument(OperationUgoiraMetadata, errors.New("illust id must be positive"))
	}
	route, routeErr := c.selectRoute(OperationUgoiraMetadata, illustID, 0)
	if routeErr != nil {
		return nil, routeErr
	}
	if route == routeWeb {
		webResult, err := c.web.UgoiraMetadata(ctx, illustID)
		if err != nil {
			return nil, mapWebError(err, OperationUgoiraMetadata, illustID)
		}
		return publicUgoiraMetadata(webResult), nil
	}
	if route != routeAppThenWeb {
		return nil, unexpectedRoute(OperationUgoiraMetadata, illustID, 0)
	}
	appResult, err := c.app.UgoiraMetadata(ctx, illustID)
	if err != nil {
		return nil, mapAppError(err, OperationUgoiraMetadata, illustID)
	}
	webResult, err := c.web.UgoiraMetadata(ctx, illustID)
	if err != nil {
		return nil, mapWebError(err, OperationUgoiraMetadata, illustID)
	}
	result := publicUgoiraMetadata(appResult)
	result.UgoiraMetadata.ZipURLs.Original = webResult.UgoiraMetadata.ZipURLs.Original
	return result, nil
}

func publicUgoiraMetadata(value *model.UgoiraMetadataResult) *UgoiraMetadataResult {
	frames := make([]UgoiraFrame, len(value.UgoiraMetadata.Frames))
	for i, frame := range value.UgoiraMetadata.Frames {
		frames[i] = UgoiraFrame{File: frame.File, Delay: frame.Delay}
	}
	return &UgoiraMetadataResult{UgoiraMetadata: UgoiraMetadata{
		ZipURLs: UgoiraZipURLs{Medium: value.UgoiraMetadata.ZipURLs.Medium, Original: value.UgoiraMetadata.ZipURLs.Original},
		Frames:  frames,
	}}
}

func cursorIllustOffset(cursor Cursor, operation Operation, digest string, illustID int64) (int, error) {
	value, err := decodeCursor(cursor, operation, digest, "offset")
	if err != nil || int64(int(value)) != value {
		return 0, newError(CodeInvalidArgument, operation, "", false, 0, illustID, errors.New("cursor is invalid"))
	}
	return int(value), nil
}

func invalidIllustArgument(operation Operation, cause error) error {
	return newError(CodeInvalidArgument, operation, "", false, 0, 0, cause)
}
