package config

import (
	"errors"
	"fmt"

	"github.com/creachadair/tomledit/parser"
	"github.com/creachadair/tomledit/transform"
	"strings"
)

// 默认账号配置读写。默认账号不进入数据库，只保存对应的非 secret UID。

const (
	pixivAuthKey   = "pixiv.auth"
	fanboxAuthKey  = "fanbox.auth"
	defaultUserKey = "default_user_id"
)

// ReadPixivDefaultUserID 返回 [pixiv.auth].default_user_id；未设置时 ok=false。
func ReadPixivDefaultUserID() (userID int64, ok bool, err error) {
	return readDefaultUserID(pixivAuthKey)
}

// SetPixivDefaultUserID 写入 [pixiv.auth].default_user_id。
func SetPixivDefaultUserID(userID int64) error {
	return writeDefaultUserID(pixivAuthKey, userID)
}

// ClearPixivDefaultUserID 删除 [pixiv.auth].default_user_id，恢复首个入库账号。
func ClearPixivDefaultUserID() error {
	return clearDefaultUserID(pixivAuthKey)
}

// ReadFanboxDefaultUserID 返回 [fanbox.auth].default_user_id；未设置时 ok=false。
func ReadFanboxDefaultUserID() (userID int64, ok bool, err error) {
	return readDefaultUserID(fanboxAuthKey)
}

// SetFanboxDefaultUserID 写入 [fanbox.auth].default_user_id。
func SetFanboxDefaultUserID(userID int64) error {
	return writeDefaultUserID(fanboxAuthKey, userID)
}

// ClearFanboxDefaultUserID 删除 [fanbox.auth].default_user_id，恢复首个入库账号。
func ClearFanboxDefaultUserID() error {
	return clearDefaultUserID(fanboxAuthKey)
}

func sectionPath(key string) []string {
	return strings.Split(key, ".")
}

func readDefaultUserID(sectionKey string) (int64, bool, error) {
	state, err := LoadSettingsState()
	if err != nil {
		return 0, false, err
	}
	raw := state.file.Get(sectionKey + "." + defaultUserKey)
	if raw == nil {
		return 0, false, nil
	}
	value, err := coercePositiveInt64(raw)
	if err != nil {
		return 0, false, fmt.Errorf("config: %s.%s must be a positive integer", sectionKey, defaultUserKey)
	}
	return value, true, nil
}

func coercePositiveInt64(raw any) (int64, error) {
	switch value := raw.(type) {
	case int64:
		if value <= 0 {
			return 0, errors.New("not positive")
		}
		return value, nil
	case int:
		if int64(value) <= 0 {
			return 0, errors.New("not positive")
		}
		return int64(value), nil
	case float64:
		if value <= 0 || value != float64(int64(value)) {
			return 0, errors.New("not a positive integer")
		}
		return int64(value), nil
	default:
		return 0, errors.New("unsupported type")
	}
}

func writeDefaultUserID(sectionKey string, userID int64) error {
	if userID <= 0 {
		return errors.New("config: default_user_id must be positive")
	}
	path, err := ConfigFilePath()
	if err != nil {
		return err
	}
	doc, err := loadConfigDocument(path)
	if err != nil {
		return err
	}
	section := ensureConfigSection(doc, sectionPath(sectionKey))
	value, err := parser.ParseValue(fmt.Sprintf("%d", userID))
	if err != nil {
		return err
	}
	if !transform.InsertMapping(section, &parser.KeyValue{Name: parser.Key{defaultUserKey}, Value: value}, true) {
		return errors.New("config: failed to update default_user_id")
	}
	return saveConfigDocument(path, doc)
}

func clearDefaultUserID(sectionKey string) error {
	path, err := ConfigFilePath()
	if err != nil {
		return err
	}
	doc, err := loadConfigDocument(path)
	if err != nil {
		return err
	}
	entry := doc.First(append(sectionPath(sectionKey), defaultUserKey)...)
	if entry == nil {
		return saveConfigDocument(path, doc)
	}
	entry.Remove()
	if sectionEntry := transform.FindTable(doc, sectionPath(sectionKey)...); sectionEntry != nil && len(sectionEntry.Section.Items) == 0 {
		sectionEntry.Remove()
	}
	return saveConfigDocument(path, doc)
}
