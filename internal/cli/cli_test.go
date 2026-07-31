package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunNoArgsPrintsHelp(t *testing.T) {
	useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), "Usage:")
	assert.Contains(t, stdout.String(), "auth")
	assert.Contains(t, stdout.String(), "config")
	assert.NotContains(t, stdout.String(), "completion")
}

func TestRunIgnoresLegacyLoggingConfiguration(t *testing.T) {
	_, configPath := useTempPaths(t)
	if err := config.WritePrivateFile(configPath, []byte("[logging]\nlevel = 'info'\n")); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run code=%d stderr=%s", code, stderr.String())
	}
	assert.Contains(t, stdout.String(), "Usage:")
	assert.Empty(t, stderr.String())
}

func TestHelpAndConfigPathSurviveBrokenNonLoggingRuntimeConfig(t *testing.T) {
	for _, body := range []string{
		"[web]\nfallback_enabled = 'not-a-bool'\n",
		"[unterminated\n",
	} {
		_, configPath := useTempPaths(t)
		if err := config.WritePrivateFile(configPath, []byte(body)); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"pixiv"}, {"pixiv", "config", "path"}} {
			var stdout, stderr bytes.Buffer
			if code := Run(args, strings.NewReader(""), &stdout, &stderr); code != 0 {
				t.Fatalf("config=%q Run(%v) code=%d stderr=%s", body, args, code, stderr.String())
			}
		}
	}
}

func TestInvalidLegacyLoggingConfigurationDoesNotBlockOtherCommands(t *testing.T) {
	_, configPath := useTempPaths(t)
	require.NoError(t, config.WritePrivateFile(configPath, []byte("[logging]\nlevel = 'loud'\n")))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "config", "path"}, strings.NewReader(""), &stdout, &stderr)

	assert.Equal(t, 0, code, stderr.String())
	assert.NotEmpty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestRunUnknownCommandReturnsError(t *testing.T) {
	_, configPath := useTempPaths(t)
	if err := config.WritePrivateFile(configPath, []byte("[logging]\nlevel = 'error'\n")); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "wat"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Contains(t, stderr.String(), `unknown command "wat"`)
	assert.NotContains(t, stderr.String(), `"level":"ERROR"`)
	assert.NotContains(t, stderr.String(), "pixiv operation")
	_, err := os.Stat(filepath.Join(filepath.Dir(configPath), "logs"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRunAccountCommandIsRemoved(t *testing.T) {
	useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "account"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Contains(t, stderr.String(), `unknown command "account"`)
}

func TestRunMCPDispatch(t *testing.T) {
	useTempPaths(t)

	old := runMCPServer
	t.Cleanup(func() { runMCPServer = old })

	called := false
	var seenProxy *string
	runMCPServer = func(_ context.Context, proxyOverride *string) error {
		called = true
		seenProxy = proxyOverride
		return nil
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "mcp", "--proxy", "http://flag-proxy"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.True(t, called)
	require.NotNil(t, seenProxy)
	assert.Equal(t, "http://flag-proxy", *seenProxy)
}

func TestRunMCPNoProxyDispatch(t *testing.T) {
	useTempPaths(t)

	old := runMCPServer
	t.Cleanup(func() { runMCPServer = old })

	var seenProxy *string
	runMCPServer = func(_ context.Context, proxyOverride *string) error {
		seenProxy = proxyOverride
		return nil
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "mcp", "--no-proxy"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	require.NotNil(t, seenProxy)
	assert.Empty(t, *seenProxy)
}

func TestRunMCPEmptyProxyDispatch(t *testing.T) {
	useTempPaths(t)

	old := runMCPServer
	t.Cleanup(func() { runMCPServer = old })

	var seenProxy *string
	runMCPServer = func(_ context.Context, proxyOverride *string) error {
		seenProxy = proxyOverride
		return nil
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "mcp", "--proxy", ""}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	require.NotNil(t, seenProxy)
	assert.Empty(t, *seenProxy)
}

func TestRunMCPDispatchError(t *testing.T) {
	useTempPaths(t)

	old := runMCPServer
	t.Cleanup(func() { runMCPServer = old })
	runMCPServer = func(context.Context, *string) error {
		return errors.New("boom")
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "mcp"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Contains(t, stderr.String(), "boom")
}

func TestRunMCPRejectsMalformedExplicitProxyWithoutLeakingSensitiveComponents(t *testing.T) {
	useTempPaths(t)
	proxy := "http://proxy-user-secret:proxy-pass-secret@proxy-host-secret.invalid/proxy-path-secret-%zz?proxy-query-secret=value"

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "mcp", "--proxy", proxy}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "invalid proxy configuration")
	for _, secret := range []string{"proxy-user-secret", "proxy-pass-secret", "proxy-host-secret", "proxy-path-secret", "proxy-query-secret"} {
		assert.NotContains(t, stderr.String(), secret)
	}
}

func TestNonNetworkCommandsRejectProxyFlags(t *testing.T) {
	tests := [][]string{
		{"pixiv", "auth", "list", "--proxy", "http://flag-proxy"},
		{"pixiv", "auth", "use", "123", "--no-proxy"},
		{"pixiv", "auth", "remove", "123", "--proxy", "http://flag-proxy"},
		{"pixiv", "config", "path", "--no-proxy"},
		{"pixiv", "config", "get", "https_proxy", "--proxy", "http://flag-proxy"},
		{"pixiv", "config", "set", "https_proxy", "http://file-proxy", "--no-proxy"},
		{"pixiv", "config", "unset", "https_proxy", "--proxy", "http://flag-proxy"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args[1:], " "), func(t *testing.T) {
			useTempPaths(t)

			var stdout, stderr bytes.Buffer
			code := Run(args, strings.NewReader(""), &stdout, &stderr)

			require.NotZero(t, code)
			assert.Contains(t, stderr.String(), "unknown flag:")
		})
	}
}
