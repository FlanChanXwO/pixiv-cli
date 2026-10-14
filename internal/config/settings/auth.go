package settings

import (
	"errors"
	"fmt"

	"reflect"
	"sync"

	"github.com/creachadair/tomledit/parser"
	"github.com/creachadair/tomledit/transform"
)

// 默认账号配置读写。默认账号不进入数据库，只保存对应的非 secret UID。

// 只缓存声明类型与路径；不缓存文件内容或选中的账号。
var defaultAccountSchema = sync.OnceValue(func() []settingSpecFromTags {
	entries, err := deriveSchemaFromTags(reflect.TypeOf(defaultAccountSelection{}))
	if err != nil {
		panic(err)
	}
	return entries
})

func defaultAccountSpec(product string) SettingSpec {
	return declaredFieldSpec(reflect.TypeOf(defaultAccountSelection{}), defaultAccountSchema(), product, "UserID")
}

// ReadPixivDefaultUserID 返回 [pixiv.auth].default_user_id；未设置时 ok=false。
func ReadPixivDefaultUserID() (userID int64, ok bool, err error) {
	return (Store{Files: defaultFileStore{}}).ReadPixivDefaultUserID()
}

// ReadPixivDefaultUserID 从注入的配置文件端口读取默认账号 UID。
func (s Store) ReadPixivDefaultUserID() (userID int64, ok bool, err error) {
	return s.readDefaultUserID(defaultAccountSpec("Pixiv"))
}

// SetPixivDefaultUserID 写入 [pixiv.auth].default_user_id。
func SetPixivDefaultUserID(userID int64) error {
	return (Store{Files: defaultFileStore{}}).SetPixivDefaultUserID(userID)
}

// SetPixivDefaultUserID 写入注入的配置文件端口。
func (s Store) SetPixivDefaultUserID(userID int64) error {
	return s.writeDefaultUserID(defaultAccountSpec("Pixiv"), userID)
}

// ClearPixivDefaultUserID 删除 [pixiv.auth].default_user_id，恢复首个入库账号。
func ClearPixivDefaultUserID() error {
	return (Store{Files: defaultFileStore{}}).ClearPixivDefaultUserID()
}

func (s Store) ClearPixivDefaultUserID() error {
	return s.clearDefaultUserID(defaultAccountSpec("Pixiv"))
}

// ReadFanboxDefaultUserID 返回 [fanbox.auth].default_user_id；未设置时 ok=false。
func ReadFanboxDefaultUserID() (userID int64, ok bool, err error) {
	return (Store{Files: defaultFileStore{}}).ReadFanboxDefaultUserID()
}

func (s Store) ReadFanboxDefaultUserID() (userID int64, ok bool, err error) {
	return s.readDefaultUserID(defaultAccountSpec("Fanbox"))
}

// SetFanboxDefaultUserID 写入 [fanbox.auth].default_user_id。
func SetFanboxDefaultUserID(userID int64) error {
	return (Store{Files: defaultFileStore{}}).SetFanboxDefaultUserID(userID)
}

func (s Store) SetFanboxDefaultUserID(userID int64) error {
	return s.writeDefaultUserID(defaultAccountSpec("Fanbox"), userID)
}

// ClearFanboxDefaultUserID 删除 [fanbox.auth].default_user_id，恢复首个入库账号。
func ClearFanboxDefaultUserID() error {
	return (Store{Files: defaultFileStore{}}).ClearFanboxDefaultUserID()
}

func (s Store) ClearFanboxDefaultUserID() error {
	return s.clearDefaultUserID(defaultAccountSpec("Fanbox"))
}

func (s Store) readDefaultUserID(spec SettingSpec) (int64, bool, error) {
	path, err := s.Path()
	if err != nil {
		return 0, false, err
	}
	files, err := s.fileStore()
	if err != nil {
		return 0, false, err
	}
	state, err := LoadSnapshotAtWithFileStore(path, files)
	if err != nil {
		return 0, false, err
	}
	raw := state.file.Get(spec.KoanfKey)
	if raw == nil {
		return 0, false, nil
	}
	value, err := coercePositiveInt64(raw)
	if err != nil {
		return 0, false, fmt.Errorf("config: %s must be a positive integer", spec.KoanfKey)
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

func (s Store) writeDefaultUserID(spec SettingSpec, userID int64) error {
	if userID <= 0 {
		return errors.New("config: default_user_id must be positive")
	}
	path, err := s.Path()
	if err != nil {
		return err
	}
	files, err := s.fileStore()
	if err != nil {
		return err
	}
	doc, err := loadConfigDocumentWithFileStore(path, files)
	if err != nil {
		return err
	}
	section := ensureConfigSection(doc, append([]string(nil), spec.Table...))
	value, err := parser.ParseValue(fmt.Sprintf("%d", userID))
	if err != nil {
		return err
	}
	if !transform.InsertMapping(section, &parser.KeyValue{Name: parser.Key{spec.Key}, Value: value}, true) {
		return errors.New("config: failed to update default_user_id")
	}
	return saveConfigDocumentWithFileStore(path, doc, files)
}

func (s Store) clearDefaultUserID(spec SettingSpec) error {
	path, err := s.Path()
	if err != nil {
		return err
	}
	files, err := s.fileStore()
	if err != nil {
		return err
	}
	doc, err := loadConfigDocumentWithFileStore(path, files)
	if err != nil {
		return err
	}
	entry := doc.First(append(append([]string(nil), spec.Table...), spec.Key)...)
	if entry == nil {
		return saveConfigDocumentWithFileStore(path, doc, files)
	}
	entry.Remove()
	if sectionEntry := transform.FindTable(doc, spec.Table...); sectionEntry != nil && len(sectionEntry.Section.Items) == 0 {
		sectionEntry.Remove()
	}
	return saveConfigDocumentWithFileStore(path, doc, files)
}
