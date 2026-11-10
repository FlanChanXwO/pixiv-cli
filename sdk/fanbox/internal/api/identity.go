package api

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"golang.org/x/net/html"
)

// User carries only the verified account identity; session cookies are excluded.
type User struct {
	UserID        int64  `json:"user_id"`
	DisplayName   string `json:"display_name"`
	CreatorID     string `json:"creator_id"`
	CreatorStatus string `json:"creator_status"`
	IsCreator     bool   `json:"is_creator"`
}

type identityWire struct {
	Context struct {
		User struct {
			UserID        json.RawMessage `json:"userId"`
			Name          string          `json:"name"`
			CreatorID     json.RawMessage `json:"creatorId"`
			CreatorStatus json.RawMessage `json:"creatorStatus"`
			IsCreator     *bool           `json:"isCreator"`
		} `json:"user"`
	} `json:"context"`
}

// CurrentUser fetches homepage metadata to verify the session. Missing metadata,
// malformed identity, and expired authentication are returned explicitly.
// operation labels errors with the public caller that initiated validation.
func (c *Client) CurrentUser(ctx context.Context, operation string) (User, error) {
	response, err := c.do(ctx, operation, WebBaseURL, acceptHTML)
	if err != nil {
		return User{}, err
	}
	document, err := readResponse(ctx, operation, response)
	if err != nil {
		return User{}, err
	}
	tokenizer := html.NewTokenizer(strings.NewReader(string(document)))
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			if tokenizer.Err() != io.EOF {
				return User{}, externalFailure(ctx, operation, "read identity metadata failed", tokenizer.Err())
			}
			return User{}, failure(operation, sdk.MalformedUpstreamResponse, "FANBOX identity metadata is missing", nil)
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			if !strings.EqualFold(token.Data, "meta") {
				continue
			}
			name, content := "", ""
			for _, attribute := range token.Attr {
				switch strings.ToLower(attribute.Key) {
				case "name":
					name = attribute.Val
				case "content":
					content = attribute.Val
				}
			}
			if strings.EqualFold(strings.TrimSpace(name), "metadata") && strings.TrimSpace(content) != "" {
				return parseIdentity(operation, content)
			}
		}
	}
}

func parseIdentity(operation, document string) (User, error) {
	malformed := func() (User, error) {
		return User{}, failure(operation, sdk.MalformedUpstreamResponse, "invalid FANBOX identity metadata", nil)
	}
	var envelope identityWire
	if json.Unmarshal([]byte(document), &envelope) != nil {
		return malformed()
	}
	source := envelope.Context.User
	id, valid := identityString(source.UserID)
	userID, err := strconv.ParseInt(id, 10, 64)
	if !valid || err != nil || userID <= 0 || strings.TrimSpace(source.Name) == "" {
		return malformed()
	}
	creatorID, valid := identityString(source.CreatorID)
	if !valid {
		return malformed()
	}
	out := User{UserID: userID, DisplayName: strings.TrimSpace(source.Name), CreatorID: creatorID, IsCreator: creatorID != ""}
	raw := strings.TrimSpace(string(source.CreatorStatus))
	if raw != "" && raw != "null" {
		var status string
		if json.Unmarshal(source.CreatorStatus, &status) == nil {
			out.CreatorStatus = strings.TrimSpace(status)
		} else {
			var enabled bool
			if json.Unmarshal(source.CreatorStatus, &enabled) != nil {
				return malformed()
			}
			out.CreatorStatus = strconv.FormatBool(enabled)
			out.IsCreator = enabled
		}
	}
	if source.IsCreator != nil {
		out.IsCreator = *source.IsCreator
	}
	return out, nil
}

func identityString(raw json.RawMessage) (string, bool) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return "", true
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return strings.TrimSpace(value), true
	}
	var number json.Number
	if json.Unmarshal(raw, &number) != nil {
		return "", false
	}
	return number.String(), true
}
