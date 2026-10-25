package atomicfile_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/atomicfile"
	"github.com/stretchr/testify/require"
)

func TestPrivateReplacementPreservesTargetOnFailure(t *testing.T) {
	for _, missingSource := range []bool{false, true} {
		t.Run(fmt.Sprint(missingSource), func(t *testing.T) {
			directory := t.TempDir()
			source, target := filepath.Join(directory, "source"), filepath.Join(directory, "target")
			require.NoError(t, os.WriteFile(target, []byte("old"), 0o600))
			want := "old"
			if !missingSource {
				require.NoError(t, os.WriteFile(source, []byte("new"), 0o600))
				want = "new"
			}
			outcome, err := atomicfile.ReplacePrivateFile(source, target)
			require.Equal(t, missingSource, err != nil)
			require.Equal(t, !missingSource, outcome.Committed)
			body, err := os.ReadFile(target)
			require.NoError(t, err)
			require.Equal(t, want, string(body))
			_, err = os.Stat(source)
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

type cancelAtEOF struct {
	io.Reader
	cancel context.CancelFunc
}

func (r cancelAtEOF) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		r.cancel()
	}
	return n, err
}

func TestAtomicWriteCancellationPreservesTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "state")
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o600))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reader := cancelAtEOF{Reader: strings.NewReader("new"), cancel: cancel}
	written, err := atomicfile.AtomicWrite(ctx, target, reader)
	require.ErrorIs(t, err, context.Canceled)
	require.EqualValues(t, 3, written)
	body, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "old", string(body))
}
