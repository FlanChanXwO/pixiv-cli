package main_test

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
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
			login = "pixiv fanbox auth import"
		}
		require.Equal(t, "error: not logged in; run \""+login+"\"\n", stderr)
	}
}

func TestEnumsBeforeAccount(t *testing.T) {
	isolatedHome(t)
	for _, args := range [][]string{
		{"ranking", "--mode", "nope"},
		{"timeline", "following", "--type", "artwork", "--restrict", "nope"},
		{"search", "cat", "--sort", "nope"},
		{"novel", "search", "cat", "--sort", "nope"},
		{"bookmark", "list", "--restrict", "nope"},
		{"bookmark", "tags", "--restrict", "nope"},
		{"bookmark", "add", "101", "--restrict", "nope"},
		{"user", "bookmarks", "7", "--restrict", "nope"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, code := runCLI(t, args...)
			require.Equal(t, 1, code)
			require.Empty(t, stdout)
			require.Contains(t, stderr, "must be")
			require.NotContains(t, stderr, "not logged in")
		})
	}
}

func TestErrorTextFailureOneLine(t *testing.T) {
	isolatedHome(t)
	for _, tc := range []struct {
		err  error
		want string
	}{
		{errors.New("first\nsecond\x1b[31m"), `first\nsecond\x1b[31m`},
		{fmt.Errorf("write: %w", errors.Join(sdk.NewError("pixiv", "Open", sdk.Unauthorized), errors.New("close failed"))), `write: not logged in; run "pixiv auth login"; close failed`},
		{sdk.NewError("pixiv", "Search", sdk.InvalidArgument), "invalid argument; check the command's --help"},
		{sdk.NewError("pixiv", "Search", sdk.InvalidArgument, sdk.WithHTTPStatus(400)), "upstream returned HTTP 400 (Bad Request); check service availability and account access before trying again"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			var stderr bytes.Buffer
			code := cli.Run(t.Context(), []string{"pixiv", "auth", "list", "--json"}, strings.NewReader(""), failedErrorOutput{tc.err}, &stderr)
			require.Equal(t, 1, code)
			require.Equal(t, "error: "+tc.want+"\n", stderr.String())
		})
	}
}

type failedErrorOutput struct{ err error }

func (f failedErrorOutput) Write([]byte) (int, error) { return 0, f.err }
