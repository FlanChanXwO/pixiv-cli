// FANBOX 创作者操作：资料、支持中/关注中的创作者列表和 featured tags。
// 请求类型、响应结构与映射在本文件；分页和游标在 pagination.go。

package fanbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/fanboxapi"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

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

// CreatorListKind selects the kind of creator list.
type CreatorListKind string

// CreatorListKind values define the supported creator lists.
const (
	CreatorListSupporting CreatorListKind = "supporting"
	CreatorListFollowing  CreatorListKind = "following"
)

// CreatorRequest selects one creator profile.
type CreatorRequest struct {
	CreatorID string
}

// CreatorsRequest lists supporting or following creators. Repeat the original
// Kind when continuing with Cursor.
type CreatorsRequest struct {
	Kind   CreatorListKind
	Cursor sdk.Cursor
}

// CreatorTagsRequest lists the tags used by one creator.
type CreatorTagsRequest struct {
	CreatorID string
}

// Creator returns one creator's profile.
func (c *Client) Creator(ctx context.Context, request CreatorRequest) (Creator, error) {
	ctx = logging.WithOperation(ctx, "Creator")
	if request.CreatorID == "" {
		return Creator{}, newError("Creator", sdk.InvalidArgument, errors.New("creator id is required"))
	}
	profile, err := c.creatorProfile(ctx, "Creator", request.CreatorID)
	if err != nil {
		return Creator{}, err
	}
	return c.mapCreator(profile)
}

// creatorProfile fetches and validates the raw profile response; resource
// re-resolution reads the same fields without building resources a second time.
func (c *Client) creatorProfile(ctx context.Context, operation, creatorID string) (creatorProfileWire, error) {
	creatorID = strings.TrimSpace(creatorID)
	if creatorID == "" {
		return creatorProfileWire{}, classifyError(operation, errors.New("FANBOX creator id is required"))
	}
	var response struct {
		Body creatorProfileWire `json:"body"`
	}
	query := url.Values{fanboxapi.ParamCreatorID: {creatorID}}
	if err := c.app.GetJSON(ctx, fanboxapi.PathCreatorGet, query, &response); err != nil {
		return creatorProfileWire{}, classifyError(operation, err)
	}
	profile := response.Body
	profile.CreatorID = strings.TrimSpace(profile.CreatorID)
	if profile.CreatorID == "" {
		profile.CreatorID = creatorID
	}
	if strings.TrimSpace(profile.User.Name) == "" {
		return creatorProfileWire{}, newError(operation, sdk.MalformedUpstreamResponse, errors.New("FANBOX creator profile is missing creator id or display name"))
	}
	return profile, nil
}

// Creators lists supporting or following creators.
func (c *Client) Creators(ctx context.Context, request CreatorsRequest) (sdk.Page[CreatorSummary], error) {
	ctx = logging.WithOperation(ctx, "Creators")
	kind := request.Kind
	if kind == "" {
		kind = CreatorListSupporting
	}
	path := ""
	switch kind {
	case CreatorListSupporting:
		path = fanboxapi.PathPlanListSupporting
	case CreatorListFollowing:
		path = fanboxapi.PathCreatorListFollowing
	default:
		return sdk.Page[CreatorSummary]{}, classifyError("Creators", errors.New("FANBOX creator list kind is invalid"))
	}
	query := url.Values{"kind": {string(kind)}}
	nextURL, err := c.continuationURL(ctx, "Creators", query, request.Cursor)
	if err != nil {
		return sdk.Page[CreatorSummary]{}, err
	}
	var body json.RawMessage
	if err := c.getListBody(ctx, path, nil, nextURL, &body, "Creators"); err != nil {
		return sdk.Page[CreatorSummary]{}, err
	}
	items, pageNextURL, err := decodeCreatorSummaries(body, kind)
	if err != nil {
		return sdk.Page[CreatorSummary]{}, classifyResponseError("Creators", err)
	}
	next, err := c.buildCursor(ctx, "Creators", query, pageNextURL)
	if err != nil {
		return sdk.Page[CreatorSummary]{}, err
	}
	return sdk.Page[CreatorSummary]{Items: items, Next: next}, nil
}

// CreatorTags lists the tags used by one creator.
func (c *Client) CreatorTags(ctx context.Context, request CreatorTagsRequest) ([]CreatorTag, error) {
	ctx = logging.WithOperation(ctx, "CreatorTags")
	if request.CreatorID == "" {
		return nil, newError("CreatorTags", sdk.InvalidArgument, errors.New("creator id is required"))
	}
	creatorID := strings.TrimSpace(request.CreatorID)
	if creatorID == "" {
		return nil, classifyError("CreatorTags", errors.New("FANBOX creator id is required"))
	}
	var response struct {
		Body json.RawMessage `json:"body"`
	}
	query := url.Values{fanboxapi.ParamCreatorID: {creatorID}}
	if err := c.app.GetJSON(ctx, fanboxapi.PathTagGetFeatured, query, &response); err != nil {
		return nil, classifyError("CreatorTags", err)
	}
	items, err := decodeFeaturedTags(response.Body)
	if err != nil {
		return nil, classifyResponseError("CreatorTags", err)
	}
	return items, nil
}

// mapCreator maps a validated profile response into the public model.
func (c *Client) mapCreator(source creatorProfileWire) (Creator, error) {
	id := source.CreatorID
	out := Creator{
		CreatorSummary:    CreatorSummary{ID: id, Name: strings.TrimSpace(source.User.Name)},
		HasAdultContent:   source.HasAdultContent,
		IsFollowing:       source.IsFollowing,
		PlanFee:           source.Plan.Fee,
		HasSupportingPlan: source.Plan.HasSupportingPlan,
	}
	if iconURL := strings.TrimSpace(source.User.IconURL); iconURL != "" {
		res, err := c.newResource("creator_icon", id, "", "", iconURL, "")
		if err != nil {
			return Creator{}, err
		}
		out.Icon = ImageResource{Resource: res}
	}
	if coverURL := strings.TrimSpace(source.CoverImageURL); coverURL != "" {
		res, err := c.newResource("creator_cover", id, "", "", coverURL, "")
		if err != nil {
			return Creator{}, err
		}
		out.Cover = ImageResource{Resource: res}
	}
	return out, nil
}

// decodeCreatorSummaries accepts either a bare array or the plans/creators
// envelope used by the supporting and following creator lists.
func decodeCreatorSummaries(raw json.RawMessage, kind CreatorListKind) ([]CreatorSummary, string, error) {
	var direct []creatorSummaryWire
	if err := json.Unmarshal(raw, &direct); err == nil && direct != nil {
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
	if err := json.Unmarshal(raw, &direct); err == nil && direct != nil {
		return mapTags(direct)
	}
	var wrapped struct {
		Tags []tagWire `json:"tags"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil || wrapped.Tags == nil {
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
