package config

import (
	"bytes"
	"io"
	"os"
	"testing"

	configapp "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testHost struct {
	in    io.Reader
	out   *bytes.Buffer
	err   *bytes.Buffer
	store configapp.Store
}

func (h testHost) Input() io.Reader           { return h.in }
func (h testHost) Output() io.Writer          { return h.out }
func (h testHost) ErrorOutput() io.Writer     { return h.err }
func (h testHost) UsageError(err error) error { return err }
func (h testHost) RequireExactArgs(count int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != count {
			return assert.AnError
		}
		return nil
	}
}
func (h testHost) ConfigService() configapp.Store { return h.store }
func (h testHost) ConfigPath() (string, error)    { return "/tmp/pixiv-config.toml", nil }

func TestNewCommandPreservesConfigSurface(t *testing.T) {
	cmd := NewCommand(testHost{in: bytes.NewReader(nil), out: &bytes.Buffer{}, err: &bytes.Buffer{}})

	require.Equal(t, "config", cmd.Name())
	assert.Equal(t, []string{"get", "path", "set", "unset"}, commandNames(cmd))
}

func TestCLIManagedAliasesComeFromSchema(t *testing.T) {
	want := []string{
		"account_pool_enabled",
		"account_pool_strategy",
		"directory_template",
		"download_path",
		"filename_template",
		"https_proxy",
		"log_format",
		"log_level",
		"request_interval",
		"reverse_search_pixiv_only",
		"reverse_search_provider",
		"saucenao_api_key",
	}
	require.Equal(t, want, configapp.CLISettingAliases())
	for _, alias := range want {
		require.Truef(t, isCLIConfigAlias(alias), "%s must be accepted by config commands", alias)
		require.Contains(t, configKeyHelp(), alias)
	}
	require.False(t, isCLIConfigAlias("output_json"))

	cmd := NewCommand(testHost{in: bytes.NewReader(nil), out: &bytes.Buffer{}, err: &bytes.Buffer{}})
	for _, child := range cmd.Commands() {
		if child.Name() == "get" || child.Name() == "set" || child.Name() == "unset" {
			require.Contains(t, child.Long, "log_level")
			require.Contains(t, child.Long, "log_format")
		}
	}
}

func TestCLIManagesLoggingAndRuntimeDownloadAliases(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	host := testHost{
		in:    bytes.NewReader(nil),
		out:   &bytes.Buffer{},
		err:   &bytes.Buffer{},
		store: configapp.Store{Files: localFileStore{path: path}},
	}

	for _, test := range []struct {
		alias string
		value string
	}{
		{alias: "directory_template", value: "{author}/{date}"},
		{alias: "request_interval", value: "2s"},
		{alias: "log_level", value: "debug"},
		{alias: "log_format", value: "json"},
	} {
		require.NoError(t, set(host, test.alias, test.value))
		host.out.Reset()
		require.NoError(t, get(host, test.alias))
		require.Equal(t, test.value+"\n", host.out.String())
		host.out.Reset()
		require.NoError(t, unset(host, test.alias))
	}
}

func TestSensitiveConfigSetRejectsArgumentValue(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	host := testHost{
		in:    bytes.NewReader(nil),
		out:   &bytes.Buffer{},
		err:   &bytes.Buffer{},
		store: configapp.Store{Files: localFileStore{path: path}},
	}
	cmd := newSetCommand(host)
	cmd.SetArgs([]string{"saucenao_api_key", "must-not-enter-argv"})

	err := cmd.Execute()
	require.EqualError(t, err, "sensitive config values must be provided through non-TTY stdin")
	_, statErr := os.Stat(path)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestSensitiveConfigSetReadsNonTTYStdinAndGetIsRedacted(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	host := testHost{
		in:    bytes.NewBufferString("private-key\n"),
		out:   &bytes.Buffer{},
		err:   &bytes.Buffer{},
		store: configapp.Store{Files: localFileStore{path: path}},
	}
	cmd := newSetCommand(host)
	cmd.SetArgs([]string{"saucenao_api_key"})

	require.NoError(t, cmd.Execute())
	require.Equal(t, "saucenao_api_key updated\n", host.out.String())
	value, err := host.store.Get("saucenao_api_key")
	require.NoError(t, err)
	require.Equal(t, "private-key", value.Value)

	host.out.Reset()
	require.NoError(t, get(host, "saucenao_api_key"))
	require.Equal(t, "<redacted>\n", host.out.String())
}

func TestSensitiveConfigGetIsRedactedEvenWhenUnset(t *testing.T) {
	host := testHost{
		in:    bytes.NewReader(nil),
		out:   &bytes.Buffer{},
		err:   &bytes.Buffer{},
		store: configapp.Store{Files: localFileStore{path: t.TempDir() + "/config.toml"}},
	}

	require.NoError(t, get(host, "saucenao_api_key"))
	require.Equal(t, "<redacted>\n", host.out.String())
}

func TestSensitiveEnvironmentOverrideNoteDoesNotExposeAValue(t *testing.T) {
	t.Setenv("SAUCENAO_API_KEY", "environment-secret")
	host := testHost{
		in:    bytes.NewBufferString("file-secret\n"),
		out:   &bytes.Buffer{},
		err:   &bytes.Buffer{},
		store: configapp.Store{Files: localFileStore{path: t.TempDir() + "/config.toml"}},
	}
	cmd := newSetCommand(host)
	cmd.SetArgs([]string{"saucenao_api_key"})

	require.NoError(t, cmd.Execute())
	require.Equal(t, "note: saucenao_api_key is currently overridden by environment; effective value remains controlled by environment\n", host.err.String())
	require.NotContains(t, host.err.String(), "environment-secret")
	require.NotContains(t, host.err.String(), "file-secret")
}

type localFileStore struct{ path string }

func (s localFileStore) Path() (string, error) { return s.path, nil }

func (s localFileStore) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (s localFileStore) WritePrivateFile(path string, body []byte) error {
	return os.WriteFile(path, body, 0o600)
}

func (s localFileStore) EnsurePrivateFile(path string, body []byte) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

func commandNames(cmd interface{ Commands() []*cobra.Command }) []string {
	commands := cmd.Commands()
	names := make([]string, 0, len(commands))
	for _, child := range commands {
		names = append(names, child.Name())
	}
	return names
}
