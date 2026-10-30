package main_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestUsageErrorExitCode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	var output, diagnostics bytes.Buffer
	code := cli.Run(t.Context(), []string{"pixiv", "search", "--unknown-flag"}, strings.NewReader(""), &output, &diagnostics)
	require.Equal(t, 2, code)
	require.Empty(t, output.String())
	require.Contains(t, diagnostics.String(), "unknown option '--unknown-flag'")
}

func TestCommandFailureLogging(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		for _, level := range []string{"info", "debug"} {
			t.Run(format+"/"+level, func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("HOME", home)
				t.Setenv("USERPROFILE", home)
				t.Setenv("PIXIV_LOG_FORMAT", format)
				t.Setenv("PIXIV_LOG_LEVEL", level)
				var output, diagnostics bytes.Buffer
				code := cli.Run(t.Context(), []string{"pixiv", "auth", "import", "abcdef", "--proxy", "http://synthetic-user:synthetic-password@127.0.0.1:1"}, strings.NewReader(""), &output, &diagnostics)
				require.Equal(t, 1, code)
				text := diagnostics.String()
				require.Contains(t, text, "error:")
				require.NotContains(t, text, "synthetic-user")
				require.NotContains(t, text, "synthetic-password")
				if level == "debug" {
					require.Contains(t, text, "Request failed")
					require.Contains(t, text, "127.0.0.1:1")
					require.Contains(t, text, "ECONNREFUSED")
				}
				if format == "text" && level == "info" {
					require.NotContains(t, text, "Command failed")
				} else {
					require.Contains(t, text, "Command failed")
					require.Contains(t, text, "call_id")
					require.Regexp(t, `origin["=:]+[" ]*pixiv/pixiv.go:[0-9]+`, text)
					if format == "json" {
						require.Contains(t, text, `"level":"ERROR"`)
					} else {
						require.Contains(t, text, "[DBUG]")
					}
				}
			})
		}
	}
}

func TestAuthUIDFailureDoesNotEchoCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PIXIV_LOG_FORMAT", "json")
	var output, diagnostics bytes.Buffer
	code := cli.Run(t.Context(), []string{"pixiv", "auth", "check"}, strings.NewReader("refresh-token-canary"), &output, &diagnostics)
	require.NotZero(t, code)
	require.Contains(t, diagnostics.String(), "invalid uid")
	require.NotContains(t, diagnostics.String(), "refresh-token-canary")
}

func TestEmptyAuthAccountsArray(t *testing.T) {
	isolatedHome(t)
	stdout, stderr, code := runCLI(t, "auth", "list", "--json")
	require.Zero(t, code, stderr)
	require.Contains(t, stdout, `"accounts": []`)
}

func TestNotLoggedInOneErrorLine(t *testing.T) {
	isolatedHome(t)
	for _, args := range [][]string{{"ranking"}, {"fanbox", "creators"}} {
		stdout, stderr, code := runCLI(t, args...)
		require.Equal(t, 1, code)
		require.Empty(t, stdout)
		login := "pixiv auth login"
		if args[0] == "fanbox" {
			login = "pixiv fanbox auth login"
		}
		require.Equal(t, "error: not logged in; run \""+login+"\"\n", stderr)
	}
}
