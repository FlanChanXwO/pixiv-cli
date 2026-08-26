package settings

import (
	"errors"
	"fmt"
	"strings"
)

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

func (s Store) Get(alias string) (SettingValue, error) {
	files, err := s.fileStore()
	if err != nil {
		return SettingValue{}, err
	}
	path, err := s.Path()
	if err != nil {
		return SettingValue{}, err
	}
	settings, err := LoadSnapshotAtWithFileStore(path, files)
	if err != nil {
		return SettingValue{}, err
	}
	value, err := settings.Effective(alias)
	if err != nil {
		return SettingValue{}, fmt.Errorf("%w. valid keys: %s", err, strings.Join(ValidSettingAliases(), ", "))
	}
	return value, nil
}

func (s Store) Set(alias, raw string) (ConfigMutationResult, error) {
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
	if spec.Sensitive {
		envRaw = ""
	}
	return ConfigMutationResult{Alias: alias, EnvOverride: envRaw, HasOverride: hasOverride}, nil
}

func (s Store) Unset(alias string) (ConfigMutationResult, error) {
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
	if spec.Sensitive {
		envRaw = ""
	}
	return ConfigMutationResult{Alias: alias, EnvOverride: envRaw, HasOverride: hasOverride}, nil
}
