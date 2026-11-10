package fanbox

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox/internal/api"
)

// CreatorRef addresses a creator on the client that created it.
type CreatorRef struct {
	ID     string `json:"id"`
	client *Client
}

// Creator returns a reference without network I/O.
func (c *Client) Creator(id string) *CreatorRef { return &CreatorRef{ID: id, client: c} }

// Creator carries a profile or a list summary. Get explicitly loads the profile.
type Creator struct {
	*CreatorRef
	creatorData
}

type creatorData struct {
	Name              string `json:"name"`
	HasAdultContent   bool   `json:"has_adult_content"`
	IsFollowing       bool   `json:"is_following"`
	Icon              Image  `json:"icon"`
	Cover             Image  `json:"cover"`
	PlanFee           int    `json:"plan_fee"`
	HasSupportingPlan bool   `json:"has_supporting_plan"`
}

// Get loads this creator's profile.
func (r *CreatorRef) Get(ctx context.Context) (*Creator, error) {
	const operation = "Creator.Get"
	if err := r.bound(operation); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(r.ID)
	body, err := r.client.getBody(ctx, operation, api.PathCreatorGet, url.Values{api.ParamCreatorID: {id}})
	if err != nil {
		return nil, err
	}
	var source creatorWire
	if json.Unmarshal(body, &source) != nil || source.User == nil || strings.TrimSpace(source.User.Name) == "" {
		return nil, newError(operation, sdk.MalformedUpstreamResponse, "incomplete FANBOX creator profile")
	}
	if strings.TrimSpace(source.CreatorID) == "" {
		source.CreatorID = id
	}
	if strings.TrimSpace(source.CreatorID) != id {
		return nil, newError(operation, sdk.MalformedUpstreamResponse, "creator ID does not match request")
	}
	return r.client.mapCreator(operation, source)
}

func (r *CreatorRef) bound(operation string) error {
	if r == nil || r.client == nil {
		return newError(operation, sdk.InvalidArgument, "creator is unbound; create a reference through Client.Creator")
	}
	if strings.TrimSpace(r.ID) == "" {
		return newError(operation, sdk.InvalidArgument, "creator ID is required")
	}
	return nil
}

// UnmarshalJSON decodes the ID and clears the earlier client binding.
func (r *CreatorRef) UnmarshalJSON(data []byte) error {
	var value struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*r = CreatorRef{ID: value.ID}
	return nil
}

// UnmarshalJSON restores profile data without retaining an earlier account binding.
func (c *Creator) UnmarshalJSON(data []byte) error {
	var value struct {
		creatorData
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*c = Creator{CreatorRef: &CreatorRef{ID: value.ID}, creatorData: value.creatorData}
	return nil
}

// CreatorTag retains a featured tag's display name and upstream link.
type CreatorTag struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Tags lists the featured tags used by this creator.
func (r *CreatorRef) Tags(ctx context.Context) ([]CreatorTag, error) {
	const operation = "Creator.Tags"
	if err := r.bound(operation); err != nil {
		return nil, err
	}
	body, err := r.client.getBody(ctx, operation, api.PathTagGetFeatured, url.Values{api.ParamCreatorID: {strings.TrimSpace(r.ID)}})
	if err != nil {
		return nil, err
	}
	source, err := decodeTags(body)
	if err != nil {
		return nil, err
	}
	items := make([]CreatorTag, len(source))
	for i, item := range source {
		name := strings.TrimSpace(item.Tag)
		if name == "" {
			return nil, newError(operation, sdk.MalformedUpstreamResponse, "FANBOX tag is missing a name")
		}
		items[i] = CreatorTag{Name: name, URL: item.URL}
	}
	return items, nil
}

// CreatorRelation selects the current account's supporting or following list.
type CreatorRelation string

const (
	// CreatorSupporting selects creators supported by the account.
	CreatorSupporting CreatorRelation = "supporting"
	// CreatorFollowing selects creators followed by the account.
	CreatorFollowing CreatorRelation = "following"
)

// CreatorsRequest resumes a relation list; the zero relation means supporting.
type CreatorsRequest struct {
	Relation CreatorRelation
	Cursor   sdk.Cursor
}

// Creators lists bound creator summaries. Repeat the relation and account when resuming.
func (c *Client) Creators(ctx context.Context, request CreatorsRequest) (sdk.Page[*Creator], error) {
	const operation = "Creators"
	relation := request.Relation
	if relation == "" {
		relation = CreatorSupporting
	}
	path := api.PathPlanListSupporting
	switch relation {
	case CreatorSupporting:
	case CreatorFollowing:
		path = api.PathCreatorListFollowing
	default:
		return sdk.Page[*Creator]{}, newError(operation, sdk.InvalidArgument, "invalid creator relation")
	}
	base := url.Values{}
	query, binding, err := c.pageQuery(ctx, operation, path, base, request.Cursor)
	if err != nil {
		return sdk.Page[*Creator]{}, err
	}
	body, err := c.getBody(ctx, operation, path, query)
	if err != nil {
		return sdk.Page[*Creator]{}, err
	}
	source, continuation, err := decodeCreators(body, relation)
	if err != nil {
		return sdk.Page[*Creator]{}, err
	}
	items := make([]*Creator, len(source))
	for i, entry := range source {
		items[i], err = c.mapCreator(operation, entry)
		if err != nil {
			return sdk.Page[*Creator]{}, err
		}
	}
	next, err := nextCursor(continuation, operation, path, base, binding)
	if err != nil {
		return sdk.Page[*Creator]{}, err
	}
	return sdk.NewPage(items, next, func(ctx context.Context) (sdk.Page[*Creator], error) {
		return c.Creators(ctx, CreatorsRequest{Relation: relation, Cursor: next})
	}), nil
}
