package config

import (
	"fmt"
	"strconv"

	"github.com/pelletier/go-toml/v2"
)

// ReadPixivDefaultUserID 只读取 Pixiv 默认账号，缺失时 ok 为 false。
func (s Store) ReadPixivDefaultUserID() (int64, bool, error) { return s.readDefaultUserID("pixiv") }

// ReadFanboxDefaultUserID 只读取 FANBOX 默认账号，缺失时 ok 为 false。
func (s Store) ReadFanboxDefaultUserID() (int64, bool, error) { return s.readDefaultUserID("fanbox") }

// SetPixivDefaultUserID 写入正整数 UID，保留其他配置。
func (s Store) SetPixivDefaultUserID(id int64) error { return s.setDefaultUserID("pixiv", id) }

// SetFanboxDefaultUserID 写入正整数 UID，保留其他配置。
func (s Store) SetFanboxDefaultUserID(id int64) error { return s.setDefaultUserID("fanbox", id) }

// ClearPixivDefaultUserID 删除 Pixiv 默认账号选择。
func (s Store) ClearPixivDefaultUserID() error { return s.Unset("pixiv.auth.default_user_id") }

// ClearFanboxDefaultUserID 删除 FANBOX 默认账号选择。
func (s Store) ClearFanboxDefaultUserID() error { return s.Unset("fanbox.auth.default_user_id") }

func (s Store) setDefaultUserID(product string, id int64) error {
	return s.Set(product+".auth.default_user_id", strconv.FormatInt(id, 10))
}

func (s Store) readDefaultUserID(product string) (int64, bool, error) {
	body, err := s.readFile()
	if err != nil {
		return 0, false, err
	}
	var file map[string]any
	if err := toml.Unmarshal(body, &file); err != nil {
		return 0, false, err
	}
	section, _ := file[product].(map[string]any)
	auth, _ := section["auth"].(map[string]any)
	raw, present := auth["default_user_id"]
	if !present {
		return 0, false, nil
	}
	var id int64
	switch n := raw.(type) {
	case int64:
		id = n
	case float64:
		id = int64(n)
		if float64(id) != n {
			id = 0
		}
	}
	if id <= 0 {
		return 0, false, fmt.Errorf("config: %s.auth.default_user_id must be a positive integer", product)
	}
	return id, true, nil
}
