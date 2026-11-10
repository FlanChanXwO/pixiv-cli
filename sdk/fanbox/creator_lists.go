package fanbox

import (
	"context"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox/internal/api"
)

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
	next, err := nextCursor(continuation, operation, path, query, binding)
	if err != nil {
		return sdk.Page[*Creator]{}, err
	}
	return sdk.NewPage(items, next, func(ctx context.Context) (sdk.Page[*Creator], error) {
		return c.Creators(ctx, CreatorsRequest{Relation: relation, Cursor: next})
	}), nil
}
