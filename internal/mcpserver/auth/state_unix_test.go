//go:build darwin || linux

package auth_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/stretchr/testify/require"
)

func TestStoreFailedAtomicWriteDoesNotPublishOwner(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "mcp-state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	before, err := os.ReadFile(store.Path)
	require.NoError(t, err)
	// 子进程限制文件写入，真实触发 AtomicWrite 的写错误，不修改主进程限制。
	child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestStoreWriteFailureHelper$")
	child.Env = append(os.Environ(), "PIXIV_TEST_MCP_WRITE_FAILURE="+store.Path)
	output, err := child.CombinedOutput()
	require.NoError(t, err, "write failure helper: %s", output)
	after, err := os.ReadFile(store.Path)
	require.NoError(t, err)
	require.True(t, bytes.Equal(before, after), "failed write changed owner")
	temps, err := filepath.Glob(filepath.Join(filepath.Dir(store.Path), ".atomic-write-*"))
	require.NoError(t, err)
	require.Empty(t, temps)
}

func TestStoreWriteFailureHelper(t *testing.T) {
	filename := os.Getenv("PIXIV_TEST_MCP_WRITE_FAILURE")
	if filename == "" {
		t.Skip("subprocess only")
	}
	signal.Ignore(syscall.SIGXFSZ)
	require.NoError(t, syscall.Setrlimit(syscall.RLIMIT_FSIZE, &syscall.Rlimit{Cur: 0, Max: 0}))
	secret, err := (auth.Store{Path: filename}).Init(t.Context(), true)
	require.True(t, errors.Is(err, syscall.EFBIG), "expected real filesystem write failure")
	require.Empty(t, secret, "failed commit published an owner secret")
}
