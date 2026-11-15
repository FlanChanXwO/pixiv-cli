package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/secretfile"
	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"
)

func TestConfigurationPriorityAndExplicitEmpty(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		environment      *string
	}{
		{"default", "", "./downloads", nil},
		{"file", "[download]\npath='./file'", "./file", nil},
		{"environment", "[download]\npath='./file'", "./env", new("./env")},
		{"empty environment", "[download]\npath='./file'", "", new("")},
		{"empty file", "[download]\npath=''", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("DOWNLOAD_PATH", "")
			require.NoError(t, os.Unsetenv("DOWNLOAD_PATH"))
			if tc.environment != nil {
				t.Setenv("DOWNLOAD_PATH", *tc.environment)
			}
			store := config.Store{Files: secretfile.ConfigFiles{}}
			path, err := store.Path()
			require.NoError(t, err)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
			require.NoError(t, os.WriteFile(path, []byte(tc.body), 0o600))
			value, present, err := store.Get("download.path")
			require.NoError(t, err)
			require.Equal(t, tc.want, value)
			require.True(t, present)
		})
	}
}

func TestDefaultTemplate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	store := config.Store{Files: secretfile.ConfigFiles{}}
	require.NoError(t, store.EnsureDefaultConfigFile())
	path, err := store.Path()
	require.NoError(t, err)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	template, err := os.ReadFile("default.toml")
	require.NoError(t, err)
	require.Equal(t, template, body)
	var decoded config.Config
	require.NoError(t, toml.Unmarshal(body, &decoded))
	require.Equal(t, config.Default(), decoded)
	existing := append([]byte("# existing user configuration\n"), body...)
	require.NoError(t, os.WriteFile(path, existing, 0o600))
	require.NoError(t, store.EnsureDefaultConfigFile())
	body, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, existing, body)
}

func TestTypeMismatchNamesKeyWithoutValue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	store := config.Store{Files: secretfile.ConfigFiles{}}
	path, err := store.Path()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	// 类型不符，以及解码器会在消息里回显的溢出数字。
	for _, value := range []string{"987654321", "99999999999999999999999999"} {
		require.NoError(t, os.WriteFile(path, []byte("[reverse_search]\nsaucenao_api_key = "+value+"\n"), 0o600))
		_, err = store.Current()
		require.ErrorContains(t, err, `line 2: key "saucenao_api_key"`)
		require.NotContains(t, err.Error(), value)
	}
	// 多行字符串内的出错行，等号左侧属于配置值。
	require.NoError(t, os.WriteFile(path, []byte("[reverse_search]\nsaucenao_api_key = \"\"\"\nSECRET_TOKEN=abc\\q\n\"\"\"\n"), 0o600))
	_, err = store.Current()
	require.ErrorContains(t, err, "config.toml line 3")
	require.NotContains(t, err.Error(), "SECRET_TOKEN")
}
