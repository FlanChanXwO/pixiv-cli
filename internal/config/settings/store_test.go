package settings_test

import (
	"errors"
	"os"
	"testing"

	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
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
	require.Equal(t, config.KeyOutputJSON, mutation.Alias)
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

// TestSensitiveMutationReportsEnvironmentOverrideWithoutItsValue keeps the
// secret out of mutation results.
func TestSensitiveMutationReportsEnvironmentOverrideWithoutItsValue(t *testing.T) {
	t.Setenv("SAUCENAO_API_KEY", "must-not-leak")
	files := &injectedFileStore{path: "injected/config.toml", files: make(map[string][]byte)}

	result, err := (config.Store{Files: files}).Set("saucenao_api_key", "file-key")
	require.NoError(t, err)
	require.True(t, result.HasOverride)
	require.Empty(t, result.EnvOverride)

	result, err = (config.Store{Files: files}).Unset("saucenao_api_key")
	require.NoError(t, err)
	require.True(t, result.HasOverride)
	require.Empty(t, result.EnvOverride)
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
	firstValue, err := first.Effective(config.KeyDownloadPath)
	require.NoError(t, err)
	require.Equal(t, "./first", firstValue.Value)

	files.files[path] = []byte("[download]\npath = \"./second\"\n")
	second, err := store.Current()
	require.NoError(t, err)
	secondValue, err := second.Effective(config.KeyDownloadPath)
	require.NoError(t, err)
	require.Equal(t, "./second", secondValue.Value)

	firstValue, err = first.Effective(config.KeyDownloadPath)
	require.NoError(t, err)
	require.Equal(t, "./first", firstValue.Value)

	t.Setenv("DOWNLOAD_PATH", "./environment-first")
	envFirst, err := store.Current()
	require.NoError(t, err)
	t.Setenv("DOWNLOAD_PATH", "./environment-second")
	envFirstValue, err := envFirst.Effective(config.KeyDownloadPath)
	require.NoError(t, err)
	require.Equal(t, "./environment-first", envFirstValue.Value)
	envSecond, err := store.Current()
	require.NoError(t, err)
	envSecondValue, err := envSecond.Effective(config.KeyDownloadPath)
	require.NoError(t, err)
	require.Equal(t, "./environment-second", envSecondValue.Value)
}

// TestRetiredKeysDoNotBlockWritesOrAdvancedTables covers the store path: a
// stale retired key is ignored, and unrelated advanced tables keep working.
func TestRemovedKeysStayClearableWhileAdvancedTablesExist(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	require.NoError(t, os.WriteFile(path, []byte(`[web]
fallback_enabled = true

[pixiv.network]
proxy_url = "http://p"

[fanbox.flaresolverr]
url = "http://solver"
`), 0o600))

	store := config.Store{Files: diskFileStore{path: path}}
	_, err := store.Set(config.KeyDownloadPath, "./x")
	require.NoError(t, err)
	_, err = store.Unset(config.Key("web_fallback_enabled"))
	require.Error(t, err, "a retired key is no longer a config alias")

	state, err := currentSnapshot(path)
	require.NoError(t, err)
	runtime, err := state.Runtime()
	require.NoError(t, err)
	require.Equal(t, "http://p", runtime.PixivNetwork.ProxyURL.Value)
	require.NotNil(t, runtime.FanboxFlareSolverr)
	require.Equal(t, "http://solver", runtime.FanboxFlareSolverr.URL)
}

// diskFileStore backs tests that need a real file for auth and document checks.
// currentSnapshot loads through the only public read entry point.
func currentSnapshot(path string) (config.Snapshot, error) {
	return (config.Store{Files: diskFileStore{path: path}}).Current()
}

type diskFileStore struct{ path string }

func (s diskFileStore) Path() (string, error) { return s.path, nil }

func (s diskFileStore) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (s diskFileStore) WritePrivateFile(path string, body []byte) error {
	return os.WriteFile(path, body, 0o600)
}

func (s diskFileStore) EnsurePrivateFile(path string, body []byte) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, body, 0o600)
}
