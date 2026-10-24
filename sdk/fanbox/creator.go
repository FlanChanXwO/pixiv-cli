// FANBOX 创作者操作：资料、支持中/关注中的创作者列表和 featured tags。
// 请求类型、响应结构与映射在本文件；游标编解码在 cursor.go。

package fanbox

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/fanboxapi"
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

// CreatorListKind values define the supported CreatorListKind filesystem.
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
	ctx = fanboxapi.WithOperation(ctx, "Creator")
	if request.CreatorID == "" {
		return Creator{}, newError("Creator", sdk.InvalidArgument, errors.New("creator id is required"))
	}
	profile, err := c.creatorProfile(ctx, request.CreatorID)
	if err != nil {
		return Creator{}, classifyError("Creator", err)
	}
	return c.mapCreator(profile)
}

// creatorProfile fetches and validates the raw profile response; resource
// re-resolution reads the same fields without building resources a second time.
func (c *Client) creatorProfile(ctx context.Context, creatorID string) (creatorProfileWire, error) {
	creatorID = strings.TrimSpace(creatorID)
	if creatorID == "" {
		return creatorProfileWire{}, errors.New("FANBOX creator id is required")
	}
	var response struct {
		Body creatorProfileWire `json:"body"`
	}
	query := url.Values{fanboxapi.ParamCreatorID: {creatorID}}
	if err := c.app.GetJSON(ctx, fanboxapi.PathCreatorGet, query, &response); err != nil {
		return creatorProfileWire{}, err
	}
	profile := response.Body
	profile.CreatorID = strings.TrimSpace(profile.CreatorID)
	if profile.CreatorID == "" {
		profile.CreatorID = creatorID
	}
	if profile.CreatorID == "" || strings.TrimSpace(profile.User.Name) == "" {
		return creatorProfileWire{}, errors.New("FANBOX creator profile is missing creator id or display name")
	}
	return profile, nil
}

// Creators lists supporting or following creators.
func (c *Client) Creators(ctx context.Context, request CreatorsRequest) (sdk.Page[CreatorSummary], error) {
	ctx = fanboxapi.WithOperation(ctx, "Creators")
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
		return sdk.Page[CreatorSummary]{}, classifyError("Creators", err)
	}
	next, err := c.buildCursor(ctx, "Creators", query, pageNextURL)
	if err != nil {
		return sdk.Page[CreatorSummary]{}, err
	}
	return sdk.Page[CreatorSummary]{Items: items, Next: next}, nil
}

// CreatorTags lists the tags used by one creator.
func (c *Client) CreatorTags(ctx context.Context, request CreatorTagsRequest) ([]CreatorTag, error) {
	ctx = fanboxapi.WithOperation(ctx, "CreatorTags")
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
		return nil, classifyError("CreatorTags", err)
	}
	return items, nil
}

// getListBody fetches one list page body, reusing a server continuation when
// the caller supplied one and sending query on the first page otherwise.
func (c *Client) getListBody(ctx context.Context, path string, query url.Values, nextURL string, body *json.RawMessage, operation string) error {
	var response struct {
		Body json.RawMessage `json:"body"`
	}
	var err error
	if strings.TrimSpace(nextURL) != "" {
		err = c.app.GetJSONURL(ctx, nextURL, &response)
	} else {
		err = c.app.GetJSON(ctx, path, query, &response)
	}
	if err != nil {
		return classifyError(operation, err)
	}
	*body = response.Body
	return nil
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
