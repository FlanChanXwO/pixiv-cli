package settings_test

import (
	"bytes"
	"errors"
	"os"
	"testing"

	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/creachadair/tomledit"
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

func TestStoreUsesInjectedFilePortForPathReadWriteAndInitialization(t *testing.T) {
	files := &injectedFileStore{path: "injected/config.toml", files: make(map[string][]byte)}
	store := config.Store{Files: files}

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

func TestGeneratedDefaultConfigIsCompactAndPreservesExistingFile(t *testing.T) {
	path := "injected/config.toml"
	files := &injectedFileStore{path: path, files: make(map[string][]byte)}
	store := config.Store{Files: files}

	require.NoError(t, store.EnsureDefaultConfigFile())
	body := files.files[path]
	_, err := tomledit.Parse(bytes.NewReader(body))
	require.NoError(t, err)
	for _, fragment := range []string{
		"[download]",
		`path = "./downloads"`,
		`filename_template = "{author} - {title}_{id}"`,
		"[output]",
		"json = false",
		"[login]",
		"open_browser = true",
		"use_after_login = false",
		"[update]",
		"check_enabled = true",
		"[logging]",
		`level = "info"`,
		`format = "text"`,
	} {
		require.Contains(t, string(body), fragment)
	}
	for _, fragment := range []string{"directory_template", "request_interval", "[network]", "[account_pool]"} {
		require.NotContains(t, string(body), fragment)
	}

	existing := []byte("# keep me\n[custom]\nvalue = true\n")
	files.files[path] = append([]byte(nil), existing...)
	require.NoError(t, store.EnsureDefaultConfigFile())
	require.Equal(t, string(existing), string(files.files[path]))
}

func TestStoreSurfacesInjectedReadError(t *testing.T) {
	files := &injectedFileStore{
		path:  "injected/config.toml",
		files: map[string][]byte{"injected/config.toml": []byte("broken")},
	}
	_, err := (config.Store{Files: files}).Get("output_json")
	require.Error(t, err)
	require.False(t, errors.Is(err, os.ErrNotExist))
}

func TestStoreRequiresExplicitFilePort(t *testing.T) {
	_, err := (config.Store{}).Get("output_json")
	require.ErrorContains(t, err, "config file store is not configured")
}

func TestStoreCurrentReturnsFreshImmutableSnapshots(t *testing.T) {
	path := "injected/config.toml"
	files := &injectedFileStore{
		path:  path,
		files: map[string][]byte{path: []byte("[download]\npath = \"./first\"\n")},
	}
	store := config.Store{Files: files}

	first, err := store.Current()
	require.NoError(t, err)
	firstValue, err := first.Effective("download_path")
	require.NoError(t, err)
	require.Equal(t, "./first", firstValue.Value)

	files.files[path] = []byte("[download]\npath = \"./second\"\n")
	second, err := store.Current()
	require.NoError(t, err)
	secondValue, err := second.Effective("download_path")
	require.NoError(t, err)
	require.Equal(t, "./second", secondValue.Value)

	firstValue, err = first.Effective("download_path")
	require.NoError(t, err)
	require.Equal(t, "./first", firstValue.Value)

	t.Setenv("DOWNLOAD_PATH", "./environment-first")
	envFirst, err := store.Current()
	require.NoError(t, err)
	t.Setenv("DOWNLOAD_PATH", "./environment-second")
	envFirstValue, err := envFirst.Effective("download_path")
	require.NoError(t, err)
	require.Equal(t, "./environment-first", envFirstValue.Value)
	envSecond, err := store.Current()
	require.NoError(t, err)
	envSecondValue, err := envSecond.Effective("download_path")
	require.NoError(t, err)
	require.Equal(t, "./environment-second", envSecondValue.Value)
}
