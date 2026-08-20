// Package creators owns FANBOX creator profile and creator-list endpoints.
package creators

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/creator"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/protocol"
)

// Transport is the narrow JSON capability required by creator endpoints.
type Transport interface {
	GetJSON(context.Context, string, any) error
}

// ListKind selects supporting or following creators.
type ListKind string

const (
	Supporting ListKind = "supporting"
	Following  ListKind = "following"
)

// ListRequest selects one creator list page. NextURL is only a server-provided
// continuation returned by an earlier Result.
type ListRequest struct {
	Kind    ListKind
	NextURL string
}

// ListResult is one normalized creator-list page.
type ListResult struct {
	Items   []creator.Summary
	NextURL string
}

// ProfileRequest selects a creator profile.
type ProfileRequest struct {
	CreatorID string
}

// Client owns creator endpoint routes and wire-to-entity conversion.
type Client struct {
	transport Transport
}

// New constructs a creator endpoint client from a narrow transport.
func New(transport Transport) *Client { return &Client{transport: transport} }

// Profile fetches one creator profile.
func (c *Client) Profile(ctx context.Context, request ProfileRequest) (creator.Creator, error) {
	if c == nil || c.transport == nil {
		return creator.Creator{}, errors.New("FANBOX creator transport is not configured")
	}
	id := strings.TrimSpace(request.CreatorID)
	if id == "" {
		return creator.Creator{}, errors.New("FANBOX creator id is required")
	}
	endpoint := protocol.APIBaseURL + "creator.get?" + url.Values{"creatorId": {id}}.Encode()
	var response struct {
		Body profileDTO `json:"body"`
	}
	if err := c.transport.GetJSON(ctx, endpoint, &response); err != nil {
		return creator.Creator{}, err
	}
	profileID := strings.TrimSpace(response.Body.CreatorID)
	if profileID == "" {
		profileID = id
	}
	displayName := strings.TrimSpace(response.Body.User.Name)
	if profileID == "" || displayName == "" {
		return creator.Creator{}, errors.New("FANBOX creator profile is missing creator id or display name")
	}
	return creator.Creator{
		ID:                profileID,
		DisplayName:       displayName,
		IconURL:           strings.TrimSpace(response.Body.User.IconURL),
		HasAdultContent:   response.Body.HasAdultContent,
		IsFollowing:       response.Body.IsFollowing,
		CoverURL:          strings.TrimSpace(response.Body.CoverImageURL),
		PlanFee:           response.Body.Plan.Fee,
		HasSupportingPlan: response.Body.Plan.HasSupportingPlan,
	}, nil
}

// List fetches supporting or following creators. A continuation is passed to
// the transport exactly as returned by FANBOX after protocol validation.
func (c *Client) List(ctx context.Context, request ListRequest) (ListResult, error) {
	if c == nil || c.transport == nil {
		return ListResult{}, errors.New("FANBOX creator transport is not configured")
	}
	kind := request.Kind
	if kind == "" {
		kind = Supporting
	}
	endpoint := strings.TrimSpace(request.NextURL)
	if endpoint == "" {
		switch kind {
		case Supporting:
			endpoint = protocol.APIBaseURL + "plan.listSupporting"
		case Following:
			endpoint = protocol.APIBaseURL + "creator.listFollowing"
		default:
			return ListResult{}, errors.New("FANBOX creator list kind is invalid")
		}
	} else if err := protocol.ValidateAPIURL(endpoint); err != nil {
		return ListResult{}, err
	}
	var response struct {
		Body json.RawMessage `json:"body"`
	}
	if err := c.transport.GetJSON(ctx, endpoint, &response); err != nil {
		return ListResult{}, err
	}
	return decodeList(response.Body, kind)
}

type profileDTO struct {
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

type creatorDTO struct {
	CreatorID string `json:"creatorId"`
}

type pageDTO struct {
	Plans    *[]creatorDTO `json:"plans"`
	Creators *[]creatorDTO `json:"creators"`
	NextURL  string        `json:"nextUrl"`
	PageURLs []string      `json:"pageUrls"`
}

func decodeList(raw json.RawMessage, kind ListKind) (ListResult, error) {
	var direct []creatorDTO
	if err := json.Unmarshal(raw, &direct); err == nil {
		items, err := mapSummaries(direct)
		return ListResult{Items: items}, err
	}
	var wrapped pageDTO
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return ListResult{}, fmt.Errorf("decode FANBOX %s creators", kind)
	}
	var source *[]creatorDTO
	switch kind {
	case Supporting:
		source = wrapped.Plans
	case Following:
		source = wrapped.Creators
	default:
		return ListResult{}, errors.New("FANBOX creator list kind is invalid")
	}
	if source == nil {
		return ListResult{}, fmt.Errorf("decode FANBOX %s creators", kind)
	}
	items, err := mapSummaries(*source)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: items, NextURL: firstContinuation(wrapped.PageURLs, wrapped.NextURL)}, nil
}

func mapSummaries(items []creatorDTO) ([]creator.Summary, error) {
	result := make([]creator.Summary, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.CreatorID)
		if id == "" {
			return nil, errors.New("FANBOX creator response includes an empty creator id")
		}
		result = append(result, creator.Summary{ID: id})
	}
	return result, nil
}

func firstContinuation(pageURLs []string, nextURL string) string {
	if len(pageURLs) > 0 {
		return pageURLs[0]
	}
	return nextURL
}
