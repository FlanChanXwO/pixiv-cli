package fanbox

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

// CreatorTag 是 creator 使用的一个 tag 的公开摘要。
type CreatorTag struct {
	Tag string `json:"tag"`
	URL string `json:"url,omitempty"`
}

// CreatorTags 读取 creator 使用的 tag 列表。
func (s *Session) CreatorTags(ctx context.Context, creatorID string) ([]CreatorTag, error) {
	if strings.TrimSpace(creatorID) == "" {
		return nil, errors.New("FANBOX creator id is required")
	}
	endpoint := apiBaseURL + "creator.getTags?" + url.Values{"creatorId": {creatorID}}.Encode()
	var response struct {
		Body json.RawMessage `json:"body"`
	}
	if err := s.getJSON(ctx, endpoint, &response); err != nil {
		return nil, err
	}
	var direct []CreatorTag
	if err := json.Unmarshal(response.Body, &direct); err == nil {
		return validateCreatorTags(direct)
	}
	var wrapped struct {
		Tags []CreatorTag `json:"tags"`
	}
	if err := json.Unmarshal(response.Body, &wrapped); err != nil {
		return nil, errors.New("decode FANBOX creator tags")
	}
	return validateCreatorTags(wrapped.Tags)
}

func validateCreatorTags(tags []CreatorTag) ([]CreatorTag, error) {
	out := make([]CreatorTag, 0, len(tags))
	for _, tag := range tags {
		if strings.TrimSpace(tag.Tag) == "" {
			return nil, errors.New("FANBOX creator tag response includes an empty tag")
		}
		out = append(out, CreatorTag{Tag: strings.TrimSpace(tag.Tag), URL: tag.URL})
	}
	return out, nil
}
