package config

import (
	"errors"
	"fmt"
	"strings"
)

// ConfigFileStore 是 TOML 配置文件的 application store；文件路径、读取、私密
// 原子写回和首次创建由 bootstrap 注入 FileStore，配置解析与 precedence 仍留在
// application/config。
type ConfigFileStore struct {
	Files FileStore
}

func (s ConfigFileStore) fileStore() (FileStore, error) {
	if s.Files == nil {
		return nil, errors.New("config file store is not configured")
	}
	return s.Files, nil
}

func (s ConfigFileStore) Path() (string, error) {
	files, err := s.fileStore()
	if err != nil {
		return "", err
	}
	return files.Path()
}

// EnsureDefaultConfigFile 在首次缺失时创建基线配置，且使用注入端口提供的
// 私密文件语义；已有文件不会被覆盖。
func (s ConfigFileStore) EnsureDefaultConfigFile() error {
	files, err := s.fileStore()
	if err != nil {
		return err
	}
	path, err := files.Path()
	if err != nil {
		return err
	}
	return files.EnsurePrivateFile(path, []byte(defaultConfigTemplate))
}

func (s ConfigFileStore) Get(alias string) (SettingValue, error) {
	files, err := s.fileStore()
	if err != nil {
		return SettingValue{}, err
	}
	path, err := s.Path()
	if err != nil {
		return SettingValue{}, err
	}
	settings, err := LoadSettingsStateAtWithFileStore(path, files)
	if err != nil {
		return SettingValue{}, err
	}
	value, err := settings.Effective(alias)
	if err != nil {
		return SettingValue{}, fmt.Errorf("%w. valid keys: %s", err, strings.Join(ValidSettingAliases(), ", "))
	}
	return value, nil
}

func (s ConfigFileStore) Set(alias, raw string) (ConfigMutationResult, error) {
	files, err := s.fileStore()
	if err != nil {
		return ConfigMutationResult{}, err
	}
	spec, ok := SettingSpecByAlias(alias)
	if !ok {
		return ConfigMutationResult{}, fmt.Errorf("unknown config key %q. valid keys: %s", alias, strings.Join(ValidSettingAliases(), ", "))
	}
	_, value, err := ParseSettingInput(alias, raw)
	if err != nil {
		return ConfigMutationResult{}, err
	}
	path, err := s.Path()
	if err != nil {
		return ConfigMutationResult{}, err
	}
	if err := SetConfigValueWithFileStore(path, alias, value, files); err != nil {
		return ConfigMutationResult{}, err
	}
	envRaw, hasOverride := EnvValue(spec)
	return ConfigMutationResult{Alias: alias, EnvOverride: envRaw, HasOverride: hasOverride}, nil
}

func (s ConfigFileStore) Unset(alias string) (ConfigMutationResult, error) {
	files, err := s.fileStore()
	if err != nil {
		return ConfigMutationResult{}, err
	}
	spec, ok := SettingSpecByAlias(alias)
	if !ok {
		return ConfigMutationResult{}, fmt.Errorf("unknown config key %q. valid keys: %s", alias, strings.Join(ValidSettingAliases(), ", "))
	}
	path, err := s.Path()
	if err != nil {
		return ConfigMutationResult{}, err
	}
	if _, err := UnsetConfigValueWithFileStore(path, alias, files); err != nil {
		return ConfigMutationResult{}, err
	}
	envRaw, hasOverride := EnvValue(spec)
	return ConfigMutationResult{Alias: alias, EnvOverride: envRaw, HasOverride: hasOverride}, nil
}
