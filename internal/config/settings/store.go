package settings

import (
	"errors"
	"fmt"
	"strings"
)

// Store 是配置 schema、文件 precedence 与稀疏写回的唯一 authority。
// Files 只提供协议无关的路径/文件机制；Store 不缓存读取结果。
type Store struct {
	Files FileStore
}

// Current 每次从当前文件与环境生成新的 Snapshot。调用方应在一次 command/tool
// execution 开始时获取一次，并把该不可变快照传给后续依赖，避免运行中途配置撕裂。
func (s Store) Current() (Snapshot, error) {
	files, err := s.fileStore()
	if err != nil {
		return Snapshot{}, err
	}
	path, err := files.Path()
	if err != nil {
		return Snapshot{}, err
	}
	return loadSnapshotAt(path, files)
}

type ConfigMutationResult struct {
	Alias       Key
	EnvOverride string
	HasOverride bool
}

func (s Store) Path() (string, error) {
	files, err := s.fileStore()
	if err != nil {
		return "", err
	}
	return files.Path()
}

// fileStore 返回注入的文件端口。缺失时明确报错，而不是退回真实文件系统：
// 配置 schema 不直接依赖具体文件系统实现。
func (s Store) fileStore() (FileStore, error) {
	if s.Files == nil {
		return nil, errors.New("config file store is not configured")
	}
	return s.Files, nil
}

// EnsureDefaultConfigFile 在首次缺失时创建基线配置，且使用注入端口提供的
// 私密文件语义；已有文件不会被覆盖。
func (s Store) EnsureDefaultConfigFile() error {
	files, err := s.fileStore()
	if err != nil {
		return err
	}
	path, err := files.Path()
	if err != nil {
		return err
	}
	body, err := generatedDefaultConfig()
	if err != nil {
		return err
	}
	return files.EnsurePrivateFile(path, body)
}

func (s Store) Get(key Key) (SettingValue, error) {
	files, err := s.fileStore()
	if err != nil {
		return SettingValue{}, err
	}
	path, err := s.Path()
	if err != nil {
		return SettingValue{}, err
	}
	settings, err := loadSnapshotAt(path, files)
	if err != nil {
		return SettingValue{}, err
	}
	value, err := settings.Effective(key)
	if err != nil {
		return SettingValue{}, fmt.Errorf("%w. valid keys: %s", err, strings.Join(aliasText(ValidSettingAliases()), ", "))
	}
	return value, nil
}

func (s Store) Set(key Key, raw string) (ConfigMutationResult, error) {
	files, err := s.fileStore()
	if err != nil {
		return ConfigMutationResult{}, err
	}
	spec, ok := SettingSpecByAlias(key)
	if !ok {
		return ConfigMutationResult{}, fmt.Errorf("unknown config key %q. valid keys: %s", key, strings.Join(aliasText(ValidSettingAliases()), ", "))
	}
	_, value, err := ParseSettingInput(key, raw)
	if err != nil {
		return ConfigMutationResult{}, err
	}
	path, err := s.Path()
	if err != nil {
		return ConfigMutationResult{}, err
	}
	if err := setConfigValue(path, key, value, files); err != nil {
		return ConfigMutationResult{}, err
	}
	envRaw, hasOverride := envValue(spec)
	if spec.Sensitive {
		envRaw = ""
	}
	return ConfigMutationResult{Alias: key, EnvOverride: envRaw, HasOverride: hasOverride}, nil
}

func (s Store) Unset(key Key) (ConfigMutationResult, error) {
	files, err := s.fileStore()
	if err != nil {
		return ConfigMutationResult{}, err
	}
	spec, ok := SettingSpecByAlias(key)
	if !ok {
		return ConfigMutationResult{}, fmt.Errorf("unknown config key %q. valid keys: %s", key, strings.Join(aliasText(ValidSettingAliases()), ", "))
	}
	path, err := s.Path()
	if err != nil {
		return ConfigMutationResult{}, err
	}
	if _, err := unsetConfigValue(path, key, files); err != nil {
		return ConfigMutationResult{}, err
	}
	envRaw, hasOverride := envValue(spec)
	if spec.Sensitive {
		envRaw = ""
	}
	return ConfigMutationResult{Alias: key, EnvOverride: envRaw, HasOverride: hasOverride}, nil
}

// aliasText 把 Key 列表转为可拼接的用户可见文本。
func aliasText(keys []Key) []string {
	text := make([]string, len(keys))
	for index, key := range keys {
		text[index] = string(key)
	}
	return text
}
