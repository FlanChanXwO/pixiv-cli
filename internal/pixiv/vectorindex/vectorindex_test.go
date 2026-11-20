package vectorindex_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/vectorindex"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

func TestInboxPersistenceAndURLInvalidation(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	recorder := vectorindex.Recorder{Directory: root}
	var a pixiv.Artwork
	require.NoError(t, json.Unmarshal([]byte(`{"id":23,"title":"private snapshot","content_type":"illust","author":{"id":11,"is_followed":true},"visible":true,"is_bookmarked":true,"pages":[{"page_index":0,"original":{"url":"https://i.pximg.net/img-original/23_p0.png"}}]}`), &a))
	require.NoError(t, recorder.Append(ctx, &a))
	inbox := filepath.Join(vectorindex.Directory(root), "inbox.ndjson")
	body, err := os.ReadFile(inbox)
	require.NoError(t, err)
	require.NotContains(t, string(body), "is_followed")
	require.NotContains(t, string(body), "is_bookmarked")
	require.NotContains(t, string(body), "visible")
	require.NoError(t, os.WriteFile(inbox, append(body, []byte("broken\x00line\n")...), 0o600))
	store, err := vectorindex.Open(ctx, root)
	require.NoError(t, err)
	defer store.Close()
	report, err := store.Merge(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, report.Merged)
	require.Equal(t, 1, report.SkippedRecords)
	rejected, err := os.ReadFile(filepath.Join(vectorindex.Directory(root), "inbox.rejected.ndjson"))
	require.NoError(t, err)
	require.Equal(t, []byte("broken\x00line\n"), rejected)
	counts, err := store.Counts(ctx, "first")
	require.NoError(t, err)
	require.Equal(t, 1, counts.Pending)
}

func TestPageURLInvalidationAndModelIsolation(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	directory := vectorindex.Directory(root)
	store, err := vectorindex.Open(ctx, root)
	require.NoError(t, err)
	defer store.Close()
	inbox := filepath.Join(directory, "inbox.ndjson")
	body := `{"recorded_at":"2026-11-20T00:00:00Z","record":{"id":"23","type":"artwork","url":"https://www.pixiv.net/artworks/23"},"pages":["https://i.pximg.net/23.png"]}` + "\n"
	require.NoError(t, os.WriteFile(inbox, []byte(body), 0o600))
	_, err = store.Merge(ctx)
	require.NoError(t, err)
	db, err := sql.Open("sqlite3", filepath.Join(directory, "index.db"))
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`INSERT INTO embedding VALUES('23',0,'first',zeroblob(8),'')`)
	require.NoError(t, err)
	counts, err := store.Counts(ctx, "second")
	require.NoError(t, err)
	require.Equal(t, 1, counts.Pending)
	body = strings.ReplaceAll(strings.ReplaceAll(body, "23.png", "23.jpg"), "00:00:00Z", "01:00:00Z")
	require.NoError(t, os.WriteFile(inbox, []byte(body), 0o600))
	_, err = store.Merge(ctx)
	require.NoError(t, err)
	counts, err = store.Counts(ctx, "first")
	require.NoError(t, err)
	require.Equal(t, 1, counts.Pending)
}

func TestMergeRetainsSourceAfterCancellation(t *testing.T) {
	root := t.TempDir()
	directory := vectorindex.Directory(root)
	require.NoError(t, os.MkdirAll(directory, 0o700))
	body := []byte(`{"recorded_at":"` + time.Now().UTC().Format(time.RFC3339Nano) + `","record":{"id":"23","type":"artwork","url":"https://www.pixiv.net/artworks/23"},"pages":["https://i.pximg.net/23.png"]}` + "\n")
	merging := filepath.Join(directory, "inbox.merging")
	require.NoError(t, os.WriteFile(merging, body, 0o600))
	store, err := vectorindex.Open(t.Context(), root)
	require.NoError(t, err)
	defer store.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = store.Merge(ctx)
	require.ErrorIs(t, err, context.Canceled)
	got, err := os.ReadFile(merging)
	require.NoError(t, err)
	require.Equal(t, body, got)
	report, err := store.Merge(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, report.Merged)
}
