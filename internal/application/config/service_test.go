package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigServiceGetSetUnsetDelegateResults(t *testing.T) {
	wantValue := SettingValue{Value: true, Text: "true", Source: "file", HasValue: true}
	wantSet := ConfigMutationResult{Alias: "output_json", EnvOverride: "PIXIV_OUTPUT_JSON", HasOverride: true}
	wantUnset := ConfigMutationResult{Alias: "output_json"}
	store := &fakeConfigStore{
		get: func(alias string) (SettingValue, error) {
			assert.Equal(t, "output_json", alias)
			return wantValue, nil
		},
		set: func(alias, raw string) (ConfigMutationResult, error) {
			assert.Equal(t, "output_json", alias)
			assert.Equal(t, "true", raw)
			return wantSet, nil
		},
		unset: func(alias string) (ConfigMutationResult, error) {
			assert.Equal(t, "output_json", alias)
			return wantUnset, nil
		},
	}
	service := ConfigService{Store: store}

	value, err := service.Get("output_json")
	require.NoError(t, err)
	assert.Equal(t, wantValue, value)
	setResult, err := service.Set("output_json", "true")
	require.NoError(t, err)
	assert.Equal(t, wantSet, setResult)
	unsetResult, err := service.Unset("output_json")
	require.NoError(t, err)
	assert.Equal(t, wantUnset, unsetResult)
}

func TestConfigServiceGetSetUnsetPropagateDependencyErrors(t *testing.T) {
	getErr := errors.New("get config dependency")
	setErr := errors.New("set config dependency")
	unsetErr := errors.New("unset config dependency")
	service := ConfigService{Store: &fakeConfigStore{
		get:   func(string) (SettingValue, error) { return SettingValue{}, getErr },
		set:   func(string, string) (ConfigMutationResult, error) { return ConfigMutationResult{}, setErr },
		unset: func(string) (ConfigMutationResult, error) { return ConfigMutationResult{}, unsetErr },
	}}

	_, err := service.Get("output_json")
	require.ErrorIs(t, err, getErr)
	_, err = service.Set("output_json", "true")
	require.ErrorIs(t, err, setErr)
	_, err = service.Unset("output_json")
	require.ErrorIs(t, err, unsetErr)
}

func TestConfigServiceGetSetUnsetRejectNilStore(t *testing.T) {
	service := ConfigService{}

	_, err := service.Get("output_json")
	require.EqualError(t, err, "config store is not configured")
	_, err = service.Set("output_json", "true")
	require.EqualError(t, err, "config store is not configured")
	_, err = service.Unset("output_json")
	require.EqualError(t, err, "config store is not configured")
}

// fakeConfigStore 只为 application 的薄委托提供可观测返回值与错误。
type fakeConfigStore struct {
	get   func(string) (SettingValue, error)
	set   func(string, string) (ConfigMutationResult, error)
	unset func(string) (ConfigMutationResult, error)
}

func (*fakeConfigStore) Path() (string, error) { return "", nil }

func (f *fakeConfigStore) Get(alias string) (SettingValue, error) {
	return f.get(alias)
}

func (f *fakeConfigStore) Set(alias, raw string) (ConfigMutationResult, error) {
	return f.set(alias, raw)
}

func (f *fakeConfigStore) Unset(alias string) (ConfigMutationResult, error) {
	return f.unset(alias)
}
