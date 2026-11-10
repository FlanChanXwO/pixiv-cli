package fanbox

import (
	"encoding/json"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

type creatorWire struct {
	CreatorID string `json:"creatorId"`
	User      *struct {
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

func (c *Client) mapCreator(operation string, source creatorWire) (*Creator, error) {
	id := strings.TrimSpace(source.CreatorID)
	if id == "" {
		return nil, newError(operation, sdk.MalformedUpstreamResponse, "FANBOX creator is missing an ID")
	}
	data := creatorData{HasAdultContent: source.HasAdultContent, IsFollowing: source.IsFollowing, PlanFee: source.Plan.Fee, HasSupportingPlan: source.Plan.HasSupportingPlan}
	if source.User != nil {
		data.Name = strings.TrimSpace(source.User.Name)
		var err error
		data.Icon, err = c.image(operation, source.User.IconURL)
		if err != nil {
			return nil, err
		}
	}
	var err error
	data.Cover, err = c.image(operation, source.CoverImageURL)
	if err != nil {
		return nil, err
	}
	return &Creator{CreatorRef: c.Creator(id), creatorData: data}, nil
}

func decodeCreators(raw json.RawMessage, relation CreatorRelation) ([]creatorWire, string, error) {
	var direct []creatorWire
	if json.Unmarshal(raw, &direct) == nil && direct != nil {
		return direct, "", nil
	}
	var page struct {
		Plans    []creatorWire `json:"plans"`
		Creators []creatorWire `json:"creators"`
		paginationWire
	}
	if json.Unmarshal(raw, &page) == nil {
		items := page.Plans
		if relation == CreatorFollowing {
			items = page.Creators
		}
		if items != nil {
			return items, page.continuation(), nil
		}
	}
	return nil, "", newError("Creators", sdk.MalformedUpstreamResponse, "missing FANBOX creator list")
}

type tagWire struct {
	Tag string `json:"tag"`
	URL string `json:"url"`
}

func decodeTags(raw json.RawMessage) ([]tagWire, error) {
	var direct []tagWire
	if json.Unmarshal(raw, &direct) == nil && direct != nil {
		return direct, nil
	}
	var page struct {
		Tags []tagWire `json:"tags"`
	}
	if json.Unmarshal(raw, &page) == nil && page.Tags != nil {
		return page.Tags, nil
	}
	return nil, newError("Creator.Tags", sdk.MalformedUpstreamResponse, "missing FANBOX tag list")
}
