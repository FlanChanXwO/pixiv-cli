//go:build darwin || linux

package auth_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/stretchr/testify/require"
)

func TestRegistrationFailedWriteDoesNotPublishClient(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	before, err := os.ReadFile(store.Path)
	require.NoError(t, err)
	child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestRegistrationWriteFailureHelper$")
	child.Env = append(os.Environ(), "PIXIV_TEST_DCR_WRITE_FAILURE="+store.Path)
	output, err := child.CombinedOutput()
	require.NoError(t, err, "write failure helper: %s", output)
	after, err := os.ReadFile(store.Path)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))
	temps, err := filepath.Glob(filepath.Join(filepath.Dir(store.Path), ".atomic-write-*"))
	require.NoError(t, err)
	require.Empty(t, temps)
}

func TestRegistrationWriteFailureHelper(t *testing.T) {
	filename := os.Getenv("PIXIV_TEST_DCR_WRITE_FAILURE")
	if filename == "" {
		t.Skip("subprocess only")
	}
	handler, err := auth.NewHandler("https://example.test", auth.Store{Path: filename})
	require.NoError(t, err)
	// 仅限制子进程文件写入；旧状态可读取，真实 AtomicWrite 会失败。
	signal.Ignore(syscall.SIGXFSZ)
	require.NoError(t, syscall.Setrlimit(syscall.RLIMIT_FSIZE, &syscall.Rlimit{Cur: 0, Max: 0}))
	request := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(`{"redirect_uris":["https://client.test/cb"]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.JSONEq(t, `{"error":"server_error"}`, response.Body.String())
}
