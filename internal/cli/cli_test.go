package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
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

func TestRunKeepsTerminalFreeOfOperationLogs(t *testing.T) {
	_, configPath := useTempPaths(t)
	if err := config.WritePrivateFile(configPath, []byte("[logging]\nlevel = 'info'\n")); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage:") || strings.Contains(stdout.String(), `"component":"cli"`) {
		t.Fatalf("stdout mixed with log: %q", stdout.String())
	}
	if strings.Contains(stderr.String(), `"component":"cli"`) || strings.Contains(stderr.String(), "pixiv operation") {
		t.Fatalf("stderr unexpectedly contains operation logs: %q", stderr.String())
	}
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

func TestInvalidLoggingConfigurationStillBlocksOtherCommands(t *testing.T) {
	_, configPath := useTempPaths(t)
	require.NoError(t, config.WritePrivateFile(configPath, []byte("[logging]\nlevel = 'loud'\n")))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "list"}, strings.NewReader(""), &stdout, &stderr)

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "error: log_level must be one of trace, debug, info, warn, error\n", stderr.String())
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
}

func TestRunClosesApplicationLogWriter(t *testing.T) {
	home, configPath := useTempPaths(t)
	require.NoError(t, config.WritePrivateFile(configPath, []byte("[logging]\nlevel = 'error'\n")))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "wat"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	// Windows 不允许临时目录清理删除仍被打开的文本日志。该断言确保 CLI 在返回前
	// 已释放根 logger 的文件 writer，而不是依赖进程退出回收句柄。
	require.NoError(t, os.RemoveAll(home))
}

func TestShouldSuggestLogDirOnlyForSpecialNonAuthFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "plain", err: errors.New("boom"), want: false},
		{name: "unauthorized", err: &sdk.Error{Code: sdk.CodeUnauthorized}, want: false},
		{name: "forbidden", err: &sdk.Error{Code: sdk.CodeForbidden}, want: false},
		{name: "upstream_unavailable", err: &sdk.Error{Code: sdk.CodeUpstreamUnavailable}, want: true},
		{name: "upstream_error", err: &sdk.Error{Code: sdk.CodeUpstreamError}, want: true},
		{name: "malformed", err: &sdk.Error{Code: sdk.CodeMalformedUpstreamResponse}, want: true},
		{name: "rate_limited", err: &sdk.Error{Code: sdk.CodeRateLimited}, want: true},
		{name: "invalid_argument", err: &sdk.Error{Code: sdk.CodeInvalidArgument}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldSuggestLogDir(tc.err); got != tc.want {
				t.Fatalf("shouldSuggestLogDir(%v)=%v want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestExitSuggestsLogDirForUpstreamErrorOnly(t *testing.T) {
	useTempPaths(t)
	var errOut bytes.Buffer
	a := app{errOut: &errOut}
	code := a.exit(&sdk.Error{Code: sdk.CodeUpstreamError, Operation: sdk.OperationSearchIllust})
	require.Equal(t, 1, code)
	assert.Contains(t, errOut.String(), "error:")
	assert.Contains(t, errOut.String(), "See log directory:")
	assert.NotContains(t, errOut.String(), "refresh_token")
	assert.NotContains(t, errOut.String(), "pixiv operation")

	errOut.Reset()
	code = a.exit(&sdk.Error{Code: sdk.CodeUnauthorized, Operation: sdk.OperationStartLogin})
	require.Equal(t, 1, code)
	assert.Contains(t, errOut.String(), "error:")
	assert.NotContains(t, errOut.String(), "See log directory:")
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

func TestRunMCPNoProxyDispatch(t *testing.T) {
	useTempPaths(t)

	old := runMCPServer
	t.Cleanup(func() { runMCPServer = old })

	var seenProxy *string
	runMCPServer = func(_ context.Context, _ io.Writer, proxyOverride *string) error {
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
