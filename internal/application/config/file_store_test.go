package config

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

type injectedFileStore struct {
	path    string
	files   map[string][]byte
	ensured int
}

func (s *injectedFileStore) Path() (string, error) { return s.path, nil }

func (s *injectedFileStore) ReadFile(path string) ([]byte, error) {
	body, ok := s.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), body...), nil
}

func (s *injectedFileStore) WritePrivateFile(path string, body []byte) error {
	s.files[path] = append([]byte(nil), body...)
	return nil
}

func (s *injectedFileStore) EnsurePrivateFile(path string, body []byte) error {
	s.ensured++
	if _, ok := s.files[path]; ok {
		return nil
	}
	s.files[path] = append([]byte(nil), body...)
	return nil
}

func TestConfigFileStoreUsesInjectedFilePortForPathReadWriteAndInitialization(t *testing.T) {
	files := &injectedFileStore{path: "injected/config.toml", files: make(map[string][]byte)}
	store := ConfigFileStore{Files: files}

	require.NoError(t, store.EnsureDefaultConfigFile())
	require.Equal(t, 1, files.ensured)
	path, err := store.Path()
	require.NoError(t, err)
	require.Equal(t, "injected/config.toml", path)

	mutation, err := store.Set("output_json", "true")
	require.NoError(t, err)
	require.Equal(t, "output_json", mutation.Alias)
	value, err := store.Get("output_json")
	require.NoError(t, err)
	require.Equal(t, true, value.Value)
	require.Equal(t, "file", value.Source)

	_, err = store.Unset("output_json")
	require.NoError(t, err)
	value, err = store.Get("output_json")
	require.NoError(t, err)
	require.Equal(t, false, value.Value)
	require.Equal(t, "default", value.Source)
}

func TestConfigFileStoreSurfacesInjectedReadError(t *testing.T) {
	files := &injectedFileStore{
		path:  "injected/config.toml",
		files: map[string][]byte{"injected/config.toml": []byte("broken")},
	}
	_, err := (ConfigFileStore{Files: files}).Get("output_json")
	require.Error(t, err)
	require.False(t, errors.Is(err, os.ErrNotExist))
}

func TestConfigFileStoreRequiresExplicitFilePort(t *testing.T) {
	_, err := (ConfigFileStore{}).Get("output_json")
	require.ErrorContains(t, err, "config file store is not configured")
}
