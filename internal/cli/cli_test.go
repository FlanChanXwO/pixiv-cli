package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

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

func TestRunUnknownCommandReturnsError(t *testing.T) {
	useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "wat"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Contains(t, stderr.String(), `unknown command "wat"`)
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
	runMCPServer = func(_ context.Context, _ io.Writer, proxyOverride *string) error {
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

func TestRunMCPNoProxyFlagIsUnknown(t *testing.T) {
	useTempPaths(t)

	old := runMCPServer
	t.Cleanup(func() { runMCPServer = old })

	called := false
	runMCPServer = func(_ context.Context, _ io.Writer, proxyOverride *string) error {
		called = true
		return nil
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "mcp", "--no-proxy"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.False(t, called)
	assert.Contains(t, stderr.String(), `unknown flag: --no-proxy`)
}

func TestRunMCPEmptyProxyDispatch(t *testing.T) {
	useTempPaths(t)

	old := runMCPServer
	t.Cleanup(func() { runMCPServer = old })

	var seenProxy *string
	runMCPServer = func(_ context.Context, _ io.Writer, proxyOverride *string) error {
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
	runMCPServer = func(context.Context, io.Writer, *string) error {
		return errors.New("boom")
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "mcp"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Contains(t, stderr.String(), "boom")
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
