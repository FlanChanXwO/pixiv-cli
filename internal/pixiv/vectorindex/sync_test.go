package vectorindex_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/vectorindex"
	"github.com/FlanChanXwO/pixiv-cli/internal/vectormodel"
	"github.com/stretchr/testify/require"
)

func TestSyncInvalidCursorRestartsOnlySelectedPhase(t *testing.T) {
	for _, phase := range []string{"initial", "head", "reconcile"} {
		t.Run(phase, func(t *testing.T) {
			f, options := newBookmarkFixture(t)
			f.lists = map[string][]int{"public": {1, 2}}
			root := t.TempDir()
			store, err := vectorindex.Open(t.Context(), root)
			require.NoError(t, err)
			defer store.Close()
			db, err := sql.Open("sqlite3", filepath.Join(vectorindex.Directory(root), "index.db"))
			require.NoError(t, err)
			defer db.Close()
			_, err = db.Exec(`INSERT INTO bookmark VALUES(11,'public','99'),(11,'private','77'),(22,'public','88'); INSERT INTO state VALUES('bookmarks/11/public','{"complete":true}'),('bookmarks/11/private','malformed'); INSERT INTO scan_seen VALUES(11,'public','reconcile','99',1)`)
			require.NoError(t, err)
			key := "bookmarks/11/public"
			if phase != "initial" {
				key += "/" + phase
			}
			_, err = db.Exec(`INSERT INTO state VALUES(?, '{"cursor":"!invalid!","complete":false}') ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key)
			require.NoError(t, err)
			options.Restrict, options.Reconcile = "public", phase == "reconcile"
			report, err := store.Sync(t.Context(), options)
			require.NoError(t, err)
			require.Equal(t, "public:0", f.calls[0])
			require.True(t, report.Bookmarks.Scans["public"].Complete)
			var untouched, previous int
			require.NoError(t, db.QueryRow(`SELECT count(*) FROM bookmark WHERE account_id=22 OR restrict='private'`).Scan(&untouched))
			require.Equal(t, 2, untouched)
			require.NoError(t, db.QueryRow(`SELECT count(*) FROM bookmark WHERE account_id=11 AND restrict='public' AND artwork_id='99'`).Scan(&previous))
			if phase == "reconcile" {
				require.Zero(t, previous)
			} else {
				require.Equal(t, 1, previous)
			}
		})
	}
}

func TestMigrationFailureKeepsOriginalVersionAndProgress(t *testing.T) {
	root := t.TempDir()
	store, err := vectorindex.Open(t.Context(), root)
	require.NoError(t, err)
	require.NoError(t, store.Close())
	db, err := sql.Open("sqlite3", filepath.Join(vectorindex.Directory(root), "index.db"))
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`PRAGMA user_version=1; INSERT INTO state VALUES('saved','original')`)
	require.NoError(t, err)
	_, err = vectorindex.Open(t.Context(), root)
	require.Error(t, err)
	var version int
	var progress string
	require.NoError(t, db.QueryRow(`PRAGMA user_version`).Scan(&version))
	require.NoError(t, db.QueryRow(`SELECT value FROM state WHERE key='saved'`).Scan(&progress))
	require.Equal(t, 1, version)
	require.Equal(t, "original", progress)
}

func TestCloudRateLimitPreservesCompletedPageAndCause(t *testing.T) {
	f, options := newBookmarkFixture(t)
	f.lists = map[string][]int{"private": {1, 2}}
	store, err := vectorindex.Open(t.Context(), t.TempDir())
	require.NoError(t, err)
	defer store.Close()
	model := options.Embedder
	options.Restrict, options.Embedder = "private", &limitedEmbedder{Embedder: model}
	report, err := store.Sync(t.Context(), options)
	require.Error(t, err)
	require.Equal(t, "embedding_rate_limited", *report.Stopped)
	var httpError *vectormodel.HTTPError
	require.ErrorAs(t, err, &httpError)
	require.Equal(t, 429, httpError.Status)
	require.Equal(t, 1, report.Index.Pages)
	require.Equal(t, 1, report.Index.Pending)
	options.Embedder = model
	report, err = store.Sync(t.Context(), options)
	require.NoError(t, err)
	require.Equal(t, 1, report.Embedded)
	require.Zero(t, report.Index.Pending)
}

type limitedEmbedder struct {
	vectorindex.Embedder
	calls int
}

func TestAllScanPhasesResumeAfterEmbeddingBatchFailure(t *testing.T) {
	for _, phase := range []string{"initial", "head", "reconcile"} {
		t.Run(phase, func(t *testing.T) {
			f, options := newBookmarkFixture(t)
			f.lists = map[string][]int{"public": {}}
			for id := 1; id <= 101; id++ {
				f.lists["public"] = append(f.lists["public"], id)
			}
			root := t.TempDir()
			store, err := vectorindex.Open(t.Context(), root)
			require.NoError(t, err)
			defer store.Close()
			if phase == "head" {
				db, err := sql.Open("sqlite3", filepath.Join(vectorindex.Directory(root), "index.db"))
				require.NoError(t, err)
				_, err = db.Exec(`INSERT INTO state VALUES('bookmarks/11/public','{"complete":true}')`)
				require.NoError(t, err)
				require.NoError(t, db.Close())
			}
			model := options.Embedder
			options.Restrict, options.Reconcile, options.Embedder = "public", phase == "reconcile", &limitedEmbedder{Embedder: model}
			report, err := store.Sync(t.Context(), options)
			require.Error(t, err)
			require.Equal(t, 100, report.Bookmarks.Pages)
			require.Equal(t, phase, report.Bookmarks.Scans["public"].Phase)
			f.calls, options.Embedder = nil, model
			report, err = store.Sync(t.Context(), options)
			require.NoError(t, err)
			require.Equal(t, "public:100", f.calls[0])
			require.True(t, report.Bookmarks.Scans["public"].Complete)
			require.Equal(t, 101, report.Index.Pages)
		})
	}
}

func (m *limitedEmbedder) Image(ctx context.Context, data []byte) ([]float32, error) {
	m.calls++
	if m.calls > 1 {
		return nil, &vectormodel.HTTPError{Status: 429}
	}
	return m.Embedder.Image(ctx, data)
}
