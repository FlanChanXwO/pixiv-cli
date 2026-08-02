package fanbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// ParseIdentityMetadataHTML 从 FANBOX 首页的 metadata meta 标签读取登录身份。
// 页面缺少 metadata、metadata 非 JSON 或 user id 无效时都会失败，绝不猜测用户身份。
func ParseIdentityMetadataHTML(document []byte) (Identity, error) {
	metadata, err := metadataContent(document)
	if err != nil {
		return Identity{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(metadata)))
	decoder.UseNumber()
	var envelope struct {
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
	if err := decoder.Decode(&envelope); err != nil {
		return Identity{}, errors.New("FANBOX metadata is not valid JSON")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Identity{}, errors.New("FANBOX metadata is not valid JSON")
	}
	userID, err := positiveJSONInt(envelope.Context.User.UserID)
	if err != nil {
		return Identity{}, errors.New("FANBOX metadata has no valid user id")
	}
	name := strings.TrimSpace(envelope.Context.User.Name)
	if name == "" {
		return Identity{}, errors.New("FANBOX metadata has no display name")
	}
	creatorID, err := optionalJSONString(envelope.Context.User.CreatorID)
	if err != nil {
		return Identity{}, errors.New("FANBOX metadata has an invalid creator id")
	}
	creatorStatus, isCreator, err := creatorState(envelope.Context.User.CreatorStatus, envelope.Context.User.IsCreator, creatorID)
	if err != nil {
		return Identity{}, errors.New("FANBOX metadata has an invalid creator status")
	}
	return Identity{
		UserID:        userID,
		DisplayName:   name,
		CreatorID:     creatorID,
		CreatorStatus: creatorStatus,
		IsCreator:     isCreator,
	}, nil
}

func metadataContent(document []byte) (string, error) {
	tokenizer := html.NewTokenizer(bytes.NewReader(document))
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			if err := tokenizer.Err(); err != nil {
				return "", errors.New("FANBOX metadata tag was not found")
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			if !strings.EqualFold(token.Data, "meta") {
				continue
			}
			name, content := "", ""
			for _, attribute := range token.Attr {
				switch {
				case strings.EqualFold(attribute.Key, "name"):
					name = attribute.Val
				case strings.EqualFold(attribute.Key, "content"):
					content = attribute.Val
				}
			}
			if strings.EqualFold(strings.TrimSpace(name), "metadata") && strings.TrimSpace(content) != "" {
				return content, nil
			}
		}
	}
}

func positiveJSONInt(raw json.RawMessage) (int64, error) {
	value, err := optionalJSONString(raw)
	if err != nil || value == "" {
		return 0, errors.New("invalid user id")
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, errors.New("invalid user id")
	}
	return parsed, nil
}

func optionalJSONString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var stringValue string
	if err := json.Unmarshal(raw, &stringValue); err == nil {
		return strings.TrimSpace(stringValue), nil
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return "", err
	}
	return number.String(), nil
}

func creatorState(raw json.RawMessage, explicit *bool, creatorID string) (string, bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		if explicit != nil {
			return "", *explicit, nil
		}
		return "", creatorID != "", nil
	}
	var stringValue string
	if err := json.Unmarshal(raw, &stringValue); err == nil {
		isCreator := creatorID != ""
		if explicit != nil {
			isCreator = *explicit
		}
		return strings.TrimSpace(stringValue), isCreator, nil
	}
	var boolValue bool
	if err := json.Unmarshal(raw, &boolValue); err != nil {
		return "", false, err
	}
	if explicit != nil {
		boolValue = *explicit
	}
	return strconv.FormatBool(boolValue), boolValue, nil
}
