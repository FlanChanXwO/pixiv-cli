// 跨领域共用的 FANBOX 响应解码与值转换。
// 单领域响应结构在 creator.go、post.go；公开实体在 models.go。

package fanbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/fanboxapi"
)

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

func mapUser(identity fanboxapi.Identity) User {
	return User{
		UserID:        identity.UserID,
		DisplayName:   identity.DisplayName,
		CreatorID:     identity.CreatorID,
		CreatorStatus: identity.CreatorStatus,
		IsCreator:     identity.IsCreator,
	}
}

func parseUTCTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, errors.New("no publish time")
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

// firstContinuation prefers the server pageUrls entry over nextUrl.
func firstContinuation(pageURLs []string, nextURL string) string {
	if len(pageURLs) > 0 {
		return pageURLs[0]
	}
	return nextURL
}
