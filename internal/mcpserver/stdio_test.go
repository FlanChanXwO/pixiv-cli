package mcpserver_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/stretchr/testify/require"
)

func TestStdioStdoutIsJSONRPC(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStdioProcess$")
	command.Env = append(os.Environ(), "PIXIV_MCP_JSONRPC_TEST_PROCESS=1")
	var diagnostics bytes.Buffer
	command.Stderr = &diagnostics
	stdin, err := command.StdinPipe()
	require.NoError(t, err)
	stdout, err := command.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, command.Start())
	defer func() {
		if t.Failed() {
			cancel()
		}
		require.NoError(t, command.Wait(), diagnostics.String())
	}()
	scanner := bufio.NewScanner(stdout)
	for index, message := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"pixiv_user_detail","arguments":{"user_id":42}}}`,
	} {
		_, err := io.WriteString(stdin, message+"\n")
		require.NoError(t, err)
		require.True(t, scanner.Scan(), diagnostics.String())
		var response map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &response))
		require.Equal(t, `"2.0"`, string(response["jsonrpc"]))
		require.Equal(t, strconv.Itoa(index+1), string(response["id"]))
		require.NotEmpty(t, response["result"])
		if index == 1 {
			require.Contains(t, string(response["result"]), `"isError":true`)
		}
	}
	require.NoError(t, stdin.Close())
	require.False(t, scanner.Scan(), "extra stdout beyond JSON-RPC responses")
	require.NoError(t, scanner.Err())
}

func TestStdioProcess(t *testing.T) {
	if os.Getenv("PIXIV_MCP_JSONRPC_TEST_PROCESS") != "1" {
		t.Skip("subprocess entry point")
	}
	server := mcpserver.New(pixiv.SDKPorts{}, pixiv.Account{}, fanbox.SDKPorts{}, nil)
	if err := mcpserver.RunStdio(t.Context(), server, os.Stdin, os.Stdout); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
