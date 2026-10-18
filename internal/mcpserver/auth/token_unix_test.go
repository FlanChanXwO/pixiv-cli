//go:build darwin || linux

package auth_test

import (
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokenWriteFailureLeavesRetryableCredentials(t *testing.T) {
	child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestTokenWriteFailureHelper$")
	child.Env = append(os.Environ(), "PIXIV_TEST_TOKEN_WRITE_FAILURE=1")
	output, err := child.CombinedOutput()
	require.NoError(t, err, "write failure helper: %s", output)
}

func TestTokenWriteFailureHelper(t *testing.T) {
	if os.Getenv("PIXIV_TEST_TOKEN_WRITE_FAILURE") != "1" {
		t.Skip("subprocess only")
	}
	signal.Ignore(syscall.SIGXFSZ)
	var limit syscall.Rlimit
	require.NoError(t, syscall.Getrlimit(syscall.RLIMIT_FSIZE, &limit))
	for _, phase := range []string{"code", "rotation", "revocation"} {
		t.Run(phase, func(t *testing.T) {
			h, store, form := directCode(t)
			if phase != "code" {
				pair := decodedToken(t, directToken(h, form))
				form = url.Values{"grant_type": {"refresh_token"}, "refresh_token": {pair["refresh_token"].(string)}, "client_id": {form.Get("client_id")}, "resource": {form.Get("resource")}}
				if phase == "revocation" {
					decodedToken(t, directToken(h, form))
				}
			}
			before, err := os.ReadFile(store.Path)
			require.NoError(t, err)
			// 保留 hard limit，以便失败后恢复写入并证明同一凭证可重试。
			require.NoError(t, syscall.Setrlimit(syscall.RLIMIT_FSIZE, &syscall.Rlimit{Cur: 0, Max: limit.Max}))
			failed := directToken(h, form)
			restoreErr := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limit)
			require.NoError(t, restoreErr)
			require.Equal(t, 500, failed.Code)
			require.JSONEq(t, `{"error":"server_error"}`, failed.Body.String())
			after, err := os.ReadFile(store.Path)
			require.NoError(t, err)
			require.True(t, string(before) == string(after), "failed transaction changed state")
			temps, err := filepath.Glob(filepath.Join(filepath.Dir(store.Path), ".atomic-write-*"))
			require.NoError(t, err)
			require.Empty(t, temps)
			response := directToken(h, form)
			if phase == "revocation" {
				require.Equal(t, 400, response.Code)
				state, err := store.Read(t.Context())
				require.NoError(t, err)
				for _, grant := range state.Grants {
					require.True(t, grant.Revoked)
				}
			} else {
				decodedToken(t, response)
			}
		})
	}
}
