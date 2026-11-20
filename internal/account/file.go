package account

import (
	"bytes"
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/secretfile"
)

type document struct {
	Schema     string
	Version    int
	LastUserID int64
	Pixiv      []Pixiv
	Fanbox     []Fanbox
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
		return document{}, false, i18n.Errorf("error.account_invalid_json")
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
		return document{}, false, i18n.Errorf("error.account_invalid_file_version")
	}
	if version > authVersion {
		return document{}, false, i18n.Errorf("error.account_version_is_newer_than_supported_version_upgrade", version, authVersion)
	}
	var file fileDocument
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return document{}, false, i18n.Errorf("error.account_invalid_account_file_fields")
	}
	doc := document{Schema: file.Schema, Version: file.Version, LastUserID: file.LastUserID}
	if file.Pixiv != nil {
		doc.Pixiv = make([]Pixiv, len(file.Pixiv))
	}
	for i, entry := range file.Pixiv {
		if entry.Pool == nil || entry.RefreshToken == nil {
			return document{}, false, i18n.Errorf("error.account_missing_pixiv_credential_or_pool_membership")
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
			return document{}, false, i18n.Errorf("error.account_missing_fanbox_credential")
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
		return i18n.Errorf("error.account_invalid_account_file_structure")
	}
	seen := make(map[int64]bool, len(d.Pixiv))
	markerFound := d.LastUserID == 0
	for _, a := range d.Pixiv {
		if a.UserID <= 0 || a.CredentialRevision <= 0 || seen[a.UserID] || a.Credential.Empty() || !utf8.Valid(a.Credential.Copy()) {
			return i18n.Errorf("error.account_invalid_or_duplicate_pixiv_account")
		}
		if !utcTime(a.CreatedAt) || !utcTime(a.UpdatedAt) || (a.PremiumCheckedAt != nil && !utcTime(*a.PremiumCheckedAt)) || (a.Pool.FrozenUntil != nil && !utcTime(*a.Pool.FrozenUntil)) {
			return i18n.Errorf("error.account_invalid_pixiv_timestamp")
		}
		seen[a.UserID] = true
		markerFound = markerFound || a.UserID == d.LastUserID
	}
	if !markerFound {
		return i18n.Errorf("error.account_pool_marker_account_is_missing")
	}
	seen = make(map[int64]bool, len(d.Fanbox))
	for _, a := range d.Fanbox {
		if a.UserID <= 0 || a.CredentialRevision <= 0 || seen[a.UserID] || a.Credential.Empty() || !utf8.Valid(a.Credential.Copy()) {
			return i18n.Errorf("error.account_invalid_or_duplicate_fanbox_account")
		}
		if !utcTime(a.CreatedAt) || !utcTime(a.UpdatedAt) || !utcTime(a.ValidatedAt) {
			return i18n.Errorf("error.account_invalid_fanbox_timestamp")
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
		return i18n.Errorf("error.account_write_file", err)
	}
	return nil
}
