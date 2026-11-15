package netdiagnostic_test

import (
	"bytes"
	"log/slog"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/netdiagnostic"
	"github.com/stretchr/testify/require"
)

func TestTransportDetailsExcludeURLCredentials(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cause := &url.Error{Op: "Get", URL: "https://canary-user:canary-password@example.test/private?token=secret", Err: &net.OpError{Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1}, Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED}}}
	logger.LogAttrs(t.Context(), slog.LevelDebug, "Request failed", netdiagnostic.Attrs(cause)...)
	require.Contains(t, output.String(), `"op":"dial"`)
	require.Contains(t, output.String(), `"net":"tcp"`)
	require.Contains(t, output.String(), `"address":"127.0.0.1:1"`)
	require.Contains(t, output.String(), `"errno":"ECONNREFUSED"`)
	for _, private := range []string{"canary-user", "canary-password", "example.test", "/private", "secret"} {
		require.NotContains(t, output.String(), private)
	}
}
