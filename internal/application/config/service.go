package config

import "errors"

type ConfigService struct {
	Store ConfigStore
}

type ConfigStore interface {
	Path() (string, error)
	Get(string) (SettingValue, error)
	Set(string, string) (ConfigMutationResult, error)
	Unset(string) (ConfigMutationResult, error)
}

type ConfigMutationResult struct {
	Alias       string
	EnvOverride string
	HasOverride bool
}

func (s ConfigService) Path() (string, error) {
	if s.Store == nil {
		return "", errors.New("config store is not configured")
	}
	return s.Store.Path()
}

func (s ConfigService) Get(alias string) (SettingValue, error) {
	if s.Store == nil {
		return SettingValue{}, errors.New("config store is not configured")
	}
	return s.Store.Get(alias)
}

func (s ConfigService) Set(alias, raw string) (ConfigMutationResult, error) {
	if s.Store == nil {
		return ConfigMutationResult{}, errors.New("config store is not configured")
	}
	return s.Store.Set(alias, raw)
}

func (s ConfigService) Unset(alias string) (ConfigMutationResult, error) {
	if s.Store == nil {
		return ConfigMutationResult{}, errors.New("config store is not configured")
	}
	return s.Store.Unset(alias)
}
