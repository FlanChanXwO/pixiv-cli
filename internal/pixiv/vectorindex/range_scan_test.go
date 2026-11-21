package vectorindex_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/vectorindex"
	"github.com/stretchr/testify/require"
)

func TestRangeSkipCheckpointRestartsInvalidCursor(t *testing.T) {
	f, options := newBookmarkFixture(t)
	f.lists, f.fail, f.failOffset = map[string][]int{"public": {1, 2, 3, 4}}, true, 1
	root := t.TempDir()
	store, err := vectorindex.Open(t.Context(), root)
	require.NoError(t, err)
	defer store.Close()
	options.Restrict, options.Range = "public", &vectorindex.Range{Offset: 2, Limit: 1}
	_, err = store.Sync(t.Context(), options)
	require.Error(t, err)
	status, err := vectorindex.ReadStatus(t.Context(), root, options.Model)
	require.NoError(t, err)
	require.Empty(t, status.Bookmarks)
	require.Len(t, status.Ranges, 1)
	db, err := sql.Open("sqlite3", filepath.Join(vectorindex.Directory(root), "index.db"))
	require.NoError(t, err)
	defer db.Close()
	for key := range status.Ranges {
		_, err = db.Exec(`UPDATE state SET value=json_set(value,'$.cursor','!invalid!') WHERE key=?`, key)
		require.NoError(t, err)
	}
	f.fail, f.calls = false, nil
	report, err := store.Sync(t.Context(), options)
	require.NoError(t, err)
	require.Equal(t, "public:0", f.calls[0])
	require.Equal(t, 1, report.Index.Pages)
	require.Equal(t, 3, report.Range.Scans["public"].Scanned)
}

func TestRangeMigrationFromVersionTwoPreservesVectorsAndRollsBack(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		root := t.TempDir()
		store, err := vectorindex.Open(t.Context(), root)
		require.NoError(t, err)
		require.NoError(t, store.Close())
		db, err := sql.Open("sqlite3", filepath.Join(vectorindex.Directory(root), "index.db"))
		require.NoError(t, err)
		_, err = db.Exec(`DROP TABLE sync_selection; PRAGMA user_version=2; INSERT INTO artwork VALUES('1','{}','2026-01-01T00:00:00Z'); INSERT INTO page VALUES('1',0,'https://i.pximg.net/1.png',''); INSERT INTO embedding VALUES('1',0,'fixture',zeroblob(8),''); INSERT INTO state VALUES('bookmarks/11/private','{"complete":true}');`)
		require.NoError(t, err)
		if conflict {
			_, err = db.Exec(`CREATE TABLE sync_selection(original TEXT); INSERT INTO sync_selection VALUES('keep')`)
			require.NoError(t, err)
		}
		store, err = vectorindex.Open(t.Context(), root)
		if conflict {
			require.Error(t, err)
			var original string
			require.NoError(t, db.QueryRow(`SELECT original FROM sync_selection`).Scan(&original))
			require.Equal(t, "keep", original)
		} else {
			require.NoError(t, err)
			counts, err := store.Counts(t.Context(), "fixture")
			require.NoError(t, err)
			require.Equal(t, 1, counts.Pages)
			require.NoError(t, store.Close())
		}
		var version int
		require.NoError(t, db.QueryRow(`PRAGMA user_version`).Scan(&version))
		if conflict {
			require.Equal(t, 2, version)
		} else {
			require.Equal(t, 3, version)
		}
		require.NoError(t, db.Close())
	}
}

func TestRangeTailEmbeddingRateLimitKeepsFinalSelection(t *testing.T) {
	f, options := newBookmarkFixture(t)
	f.lists = map[string][]int{"private": {1, 2, 3, 4}}
	store, err := vectorindex.Open(t.Context(), t.TempDir())
	require.NoError(t, err)
	defer store.Close()
	options.Restrict, options.Range = "private", &vectorindex.Range{Tail: 2}
	model := options.Embedder
	options.Embedder = &limitedEmbedder{Embedder: model}
	report, err := store.Sync(t.Context(), options)
	require.Error(t, err)
	require.Equal(t, "embedding_rate_limited", *report.Stopped)
	require.Equal(t, 2, report.Index.Artworks)
	require.Equal(t, 1, report.Index.Pages)
	options.Embedder, f.calls = model, nil
	report, err = store.Sync(t.Context(), options)
	require.NoError(t, err)
	require.Empty(t, f.calls)
	require.Equal(t, 1, report.Embedded)
	require.Equal(t, 2, report.Range.Scans["private"].Matched)
}

func TestRangeResetRollsBackCandidateDeletion(t *testing.T) {
	f, options := newBookmarkFixture(t)
	f.lists, f.fail, f.failOffset = map[string][]int{"public": {1, 2, 3}}, true, 2
	root := t.TempDir()
	store, err := vectorindex.Open(t.Context(), root)
	require.NoError(t, err)
	defer store.Close()
	options.Restrict, options.Range = "public", &vectorindex.Range{Tail: 2}
	_, err = store.Sync(t.Context(), options)
	require.Error(t, err)
	db, err := sql.Open("sqlite3", filepath.Join(vectorindex.Directory(root), "index.db"))
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`UPDATE state SET value=json_set(value,'$.cursor','!invalid!'); CREATE TRIGGER reject_checkpoint BEFORE UPDATE ON state BEGIN SELECT RAISE(ABORT,'checkpoint rejected'); END;`)
	require.NoError(t, err)
	f.fail = false
	_, err = store.Sync(t.Context(), options)
	require.ErrorContains(t, err, "checkpoint rejected")
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM sync_selection`).Scan(&count))
	require.Equal(t, 2, count)
}
