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
	return LoadSnapshotAtWithFileStore(path, files)
}

type ConfigMutationResult struct {
	Alias       string
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
