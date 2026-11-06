package account

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/secretfile"
)

type document struct {
	Schema     string   `json:"schema"`
	Version    int      `json:"version"`
	LastUserID int64    `json:"pixiv_pool_last_user_id"`
	Pixiv      []Pixiv  `json:"pixiv"`
	Fanbox     []Fanbox `json:"fanbox"`
}

// 凭据只在文件编码的边界显式展开；账号值的普通 JSON、fmt 与 slog 都保持脱敏。
type pixivFile struct {
	Pixiv
	Pool         *Membership `json:"pool"`
	RefreshToken *string     `json:"refresh_token"`
}
type fanboxFile struct {
	Fanbox
	SessionID *string `json:"session_id"`
}
type fileDocument struct {
	Schema     string       `json:"schema"`
	Version    int          `json:"version"`
	LastUserID int64        `json:"pixiv_pool_last_user_id"`
	Pixiv      []pixivFile  `json:"pixiv"`
	Fanbox     []fanboxFile `json:"fanbox"`
}

func emptyDocument() document {
	return document{Schema: authSchema, Version: authVersion, Pixiv: []Pixiv{}, Fanbox: []Fanbox{}}
}

func decodeDocument(body []byte, missing bool) (document, bool, error) {
	if missing {
		return document{}, true, nil
	}
	if !utf8.Valid(body) || !json.Valid(body) {
		return document{}, false, errors.New("account: invalid JSON")
	}
	var identity map[string]json.RawMessage
	// 合法 JSON 的非对象根节点没有本格式的 schema，按首次迁移处理。
	if err := json.Unmarshal(body, &identity); err != nil {
		return document{}, true, nil
	}
	var schema string
	if err := json.Unmarshal(identity["schema"], &schema); err != nil || schema != authSchema {
		return document{}, true, nil
	}
	var version int
	if err := json.Unmarshal(identity["version"], &version); err != nil || version < 1 {
		return document{}, false, errors.New("account: invalid file version")
	}
	if version > authVersion {
		return document{}, false, fmt.Errorf("account: version %d is newer than supported version %d; upgrade pixiv-cli", version, authVersion)
	}
	var file fileDocument
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return document{}, false, errors.New("account: invalid account file fields")
	}
	doc := document{Schema: file.Schema, Version: file.Version, LastUserID: file.LastUserID}
	if file.Pixiv != nil {
		doc.Pixiv = make([]Pixiv, len(file.Pixiv))
	}
	for i, entry := range file.Pixiv {
		if entry.Pool == nil || entry.RefreshToken == nil {
			return document{}, false, errors.New("account: missing Pixiv credential or pool membership")
		}
		doc.Pixiv[i] = entry.Pixiv
		doc.Pixiv[i].Pool = *entry.Pool
		doc.Pixiv[i].Credential = credential.New([]byte(*entry.RefreshToken))
	}
	if file.Fanbox != nil {
		doc.Fanbox = make([]Fanbox, len(file.Fanbox))
	}
	for i, entry := range file.Fanbox {
		if entry.SessionID == nil {
			return document{}, false, errors.New("account: missing FANBOX credential")
		}
		doc.Fanbox[i] = entry.Fanbox
		doc.Fanbox[i].Credential = credential.New([]byte(*entry.SessionID))
	}
	if err := doc.validate(); err != nil {
		return document{}, false, err
	}
	return doc, false, nil
}

func (d document) validate() error {
	if d.Schema != authSchema || d.Version != authVersion || d.Pixiv == nil || d.Fanbox == nil {
		return errors.New("account: invalid account file structure")
	}
	seen := make(map[int64]bool, len(d.Pixiv))
	markerFound := d.LastUserID == 0
	for _, a := range d.Pixiv {
		if a.UserID <= 0 || a.CredentialRevision <= 0 || seen[a.UserID] || !utf8.Valid(a.Credential.Copy()) {
			return errors.New("account: invalid or duplicate Pixiv account")
		}
		if !utcTime(a.CreatedAt) || !utcTime(a.UpdatedAt) || (a.PremiumCheckedAt != nil && !utcTime(*a.PremiumCheckedAt)) || (a.Pool.FrozenUntil != nil && !utcTime(*a.Pool.FrozenUntil)) {
			return errors.New("account: invalid Pixiv timestamp")
		}
		seen[a.UserID] = true
		markerFound = markerFound || a.UserID == d.LastUserID
	}
	if !markerFound {
		return errors.New("account: pool marker account is missing")
	}
	seen = make(map[int64]bool, len(d.Fanbox))
	for _, a := range d.Fanbox {
		if a.UserID <= 0 || a.CredentialRevision <= 0 || seen[a.UserID] || !utf8.Valid(a.Credential.Copy()) {
			return errors.New("account: invalid or duplicate FANBOX account")
		}
		if !utcTime(a.CreatedAt) || !utcTime(a.UpdatedAt) || !utcTime(a.ValidatedAt) {
			return errors.New("account: invalid FANBOX timestamp")
		}
		seen[a.UserID] = true
	}
	return nil
}

func utcTime(t time.Time) bool { _, offset := t.Zone(); return !t.IsZero() && offset == 0 }

func (s *Store) write(doc document) error {
	if err := doc.validate(); err != nil {
		return err
	}
	file := fileDocument{Schema: doc.Schema, Version: doc.Version, LastUserID: doc.LastUserID, Pixiv: make([]pixivFile, len(doc.Pixiv)), Fanbox: make([]fanboxFile, len(doc.Fanbox))}
	for i, a := range doc.Pixiv {
		value := a.Credential.Value()
		file.Pixiv[i] = pixivFile{Pixiv: a, Pool: &a.Pool, RefreshToken: &value}
	}
	for i, a := range doc.Fanbox {
		value := a.Credential.Value()
		file.Fanbox[i] = fanboxFile{a, &value}
	}
	if err := secretfile.WriteJSON(s.path, file); err != nil {
		return fmt.Errorf("account: write file: %w", err)
	}
	return nil
}
