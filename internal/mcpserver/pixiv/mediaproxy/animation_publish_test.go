package mediaproxy

import (
	"context"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPublishedAnimationDownloadsExactBytesWithoutEncoding(t *testing.T) {
	for _, format := range []string{"gif", "apng"} {
		t.Run(format, func(t *testing.T) {
			s, ctx, calls, version, _ := animationProxy(t, 1<<20)
			first, err := s.PrepareAnimation(ctx, 72, 42, format)
			require.NoError(t, err)
			ready := awaitAnimation(t, s, ctx, first.Handle)
			get := func(url string) []byte {
				w := httptest.NewRecorder()
				s.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
				require.Equal(t, 200, w.Code)
				return w.Body.Bytes()
			}
			data, preview := get(ready.AnimationURL), get(ready.PreviewURL)
			before := calls.Load()
			sourceSession, err := s.session(ctx, 72)
			require.NoError(t, err)
			source := sourceSession.Lease.Value()
			require.NoError(t, sourceSession.Lease.Close())
			published, err := s.PublishAnimation(ctx, 72, 42, format, source, data, preview)
			require.NoError(t, err, "complete inline bytes must have a protected browser download")
			require.Equal(t, "ready", published.State)
			require.Equal(t, data, get(published.AnimationURL+"?download=1"))
			require.Equal(t, preview, get(published.PreviewURL))
			again, err := s.PublishAnimation(ctx, 72, 42, format, source, data, preview)
			require.NoError(t, err)
			require.Equal(t, published.AnimationURL, again.AnimationURL)
			require.Equal(t, before, calls.Load(), "publishing must not download or encode again")
			s.animations.mu.Lock()
			var filenames []string
			for _, job := range s.animations.jobs {
				if job.id != "" {
					filenames = append(filenames, filepath.Join(job.dir, "animation."+format))
				}
			}
			s.animations.mu.Unlock()
			for _, filename := range filenames {
				info, statErr := os.Stat(filename)
				require.NoError(t, statErr)
				// Windows file modes do not expose POSIX permission bits.
				if runtime.GOOS != "windows" {
					require.Equal(t, os.FileMode(0600), info.Mode().Perm())
				}
			}
			small, smallCtx, _, _, _ := animationProxy(t, int64(len(data)+len(preview)-1))
			smallSession, err := small.session(smallCtx, 72)
			require.NoError(t, err)
			smallSource := smallSession.Lease.Value()
			require.NoError(t, smallSession.Lease.Close())
			baseline, err := os.ReadDir(small.animations.root)
			require.NoError(t, err)
			_, err = small.PublishAnimation(smallCtx, 72, 42, format, smallSource, data, preview)
			require.ErrorIs(t, err, errAnimationCapacity)
			require.Zero(t, small.animations.used)
			files, err := os.ReadDir(small.animations.root)
			require.NoError(t, err)
			require.Equal(t, baseline, files, "capacity rejection must retain only the existing workspace ownership files")
			cancelled, cancel := context.WithCancel(ctx)
			cancel()
			_, err = s.PublishAnimation(cancelled, 72, 42, format, source, data, preview)
			require.Error(t, err)
			_, err = s.PublishAnimation(context.Background(), 72, 42, format, source, data, preview)
			require.Error(t, err, "unauthenticated callers cannot publish download capabilities")

			stale, err := pixiv.NewWith("fixture-stale", pixiv.Options{})
			require.NoError(t, err)
			_, err = s.PublishAnimation(ctx, 72, 42, format, stale, data, preview)
			require.Error(t, err, "a previous read snapshot must not acquire the current account version")
			version.Add(1)
			w := httptest.NewRecorder()
			s.ServeHTTP(w, httptest.NewRequest("GET", published.AnimationURL, nil))
			require.Equal(t, 401, w.Code)
		})
	}
}
