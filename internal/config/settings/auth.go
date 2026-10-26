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

// ReadPixivDefaultUserID 从注入的配置文件端口读取默认账号 UID。
func (s Store) ReadPixivDefaultUserID() (userID int64, ok bool, err error) {
	return s.readDefaultUserID(defaultAccountSpec("Pixiv"))
}

// SetPixivDefaultUserID 写入注入的配置文件端口。
func (s Store) SetPixivDefaultUserID(userID int64) error {
	return s.writeDefaultUserID(defaultAccountSpec("Pixiv"), userID)
}

func (s Store) ClearPixivDefaultUserID() error {
	return s.clearDefaultUserID(defaultAccountSpec("Pixiv"))
}

func (s Store) ReadFanboxDefaultUserID() (userID int64, ok bool, err error) {
	return s.readDefaultUserID(defaultAccountSpec("Fanbox"))
}

func (s Store) SetFanboxDefaultUserID(userID int64) error {
	return s.writeDefaultUserID(defaultAccountSpec("Fanbox"), userID)
}

func (s Store) ClearFanboxDefaultUserID() error {
	return s.clearDefaultUserID(defaultAccountSpec("Fanbox"))
}

func (s Store) readDefaultUserID(spec SettingSpec) (int64, bool, error) {
	files, err := s.fileStore()
	if err != nil {
		return 0, false, err
	}
	path, err := files.Path()
	if err != nil {
		return 0, false, err
	}
	state, err := loadSnapshot(path, files)
	if err != nil {
		return 0, false, err
	}
	raw, present := state.fileValue(spec)
	if !present {
		return 0, false, nil
	}
	value, err := coercePositiveInt64(raw)
	if err != nil {
		return 0, false, fmt.Errorf("config: %s must be a positive integer", spec.TOMLPath)
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
	files, err := s.fileStore()
	if err != nil {
		return err
	}
	path, err := files.Path()
	if err != nil {
		return err
	}
	doc, err := loadConfigDocument(path, files)
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
	return saveConfigDocument(path, doc, files)
}

func (s Store) clearDefaultUserID(spec SettingSpec) error {
	files, err := s.fileStore()
	if err != nil {
		return err
	}
	path, err := files.Path()
	if err != nil {
		return err
	}
	doc, err := loadConfigDocument(path, files)
	if err != nil {
		return err
	}
	entry := doc.First(append(append([]string(nil), spec.Table...), spec.Key)...)
	if entry == nil {
		return saveConfigDocument(path, doc, files)
	}
	entry.Remove()
	if sectionEntry := transform.FindTable(doc, spec.Table...); sectionEntry != nil && len(sectionEntry.Section.Items) == 0 {
		sectionEntry.Remove()
	}
	return saveConfigDocument(path, doc, files)
}
