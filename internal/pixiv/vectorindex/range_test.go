package vectorindex_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/searchfilter"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/vectorindex"
	"github.com/stretchr/testify/require"
)

func TestRangeSelectionUsesOriginalPositionsPerVisibility(t *testing.T) {
	for _, tc := range []struct {
		name            string
		rangeOf         vectorindex.Range
		public, private int
	}{
		{"head", vectorindex.Range{Limit: 2}, 2, 2},
		{"middle", vectorindex.Range{Offset: 1, Limit: 2}, 2, 2},
		{"tail", vectorindex.Range{Tail: 2}, 2, 2},
		{"offset", vectorindex.Range{Offset: 3}, 2, 0},
		{"beyond", vectorindex.Range{Offset: 10, Limit: 2}, 0, 0},
		{"all", vectorindex.Range{}, 5, 3},
		{"short_tail", vectorindex.Range{Tail: 20}, 5, 3},
		{"empty", vectorindex.Range{Tail: 2}, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, options := newBookmarkFixture(t)
			f.pageSize, f.lists = 3, map[string][]int{"public": {1, 2, 3, 4, 5}, "private": {6, 7, 8}}
			if tc.name == "empty" {
				f.lists = nil
			}
			store, err := vectorindex.Open(t.Context(), t.TempDir())
			require.NoError(t, err)
			defer store.Close()
			options.Range = &tc.rangeOf
			report, err := store.Sync(t.Context(), options)
			require.NoError(t, err)
			require.Equal(t, tc.public+tc.private, report.Index.Pages)
			for scope, n := range map[string]int{"public": tc.public, "private": tc.private} {
				require.Equal(t, n, report.Range.Scans[scope].Selected)
				require.Equal(t, vectorindex.RangeComplete, report.Range.Scans[scope].Phase)
			}
			require.Empty(t, report.Bookmarks.Scans)
		})
	}
}

func TestTailResumeKeepsCandidatesPrivateUntilEnd(t *testing.T) {
	f, options := newBookmarkFixture(t)
	f.lists, f.fail, f.failOffset = map[string][]int{"public": {1, 2, 3, 4}}, true, 2
	root := t.TempDir()
	store, err := vectorindex.Open(t.Context(), root)
	require.NoError(t, err)
	options.Restrict, options.Range = "public", &vectorindex.Range{Tail: 2}
	report, err := store.Sync(t.Context(), options)
	require.Error(t, err)
	require.Zero(t, report.Index.Artworks)
	require.Equal(t, 2, report.Range.Scans["public"].Scanned)
	require.NoError(t, store.Close())
	store, err = vectorindex.Open(t.Context(), root)
	require.NoError(t, err)
	defer store.Close()
	f.fail, f.calls = false, nil
	report, err = store.Sync(t.Context(), options)
	require.NoError(t, err)
	require.Equal(t, "public:2", f.calls[0])
	require.Equal(t, 2, report.Index.Pages)
	f.lists["public"], f.calls = []int{1, 2, 3, 4, 5}, nil
	report, err = store.Sync(t.Context(), options)
	require.NoError(t, err)
	require.Equal(t, "public:0", f.calls[0])
	require.Equal(t, 1, report.Range.Scans["public"].ReusedPages)
	require.Equal(t, 3, report.Index.Artworks)
}

func TestRangeCancellationResumesEmbeddingWithoutRescanning(t *testing.T) {
	f, options := newBookmarkFixture(t)
	f.lists = map[string][]int{"private": {1, 2, 3, 4}}
	store, err := vectorindex.Open(t.Context(), t.TempDir())
	require.NoError(t, err)
	defer store.Close()
	options.Restrict, options.Range = "private", &vectorindex.Range{Limit: 3}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	options.Progress = func(int, int) error { cancel(); return ctx.Err() }
	report, err := store.Sync(ctx, options)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, report.Index.Pages)
	f.calls, options.Progress = nil, nil
	report, err = store.Sync(t.Context(), options)
	require.NoError(t, err)
	require.Empty(t, f.calls)
	require.Equal(t, 2, report.Embedded)
	require.Equal(t, 3, report.Index.Pages)
}

func TestRangeTailBatchResumeSkipsCompletedVisibility(t *testing.T) {
	f, options := newBookmarkFixture(t)
	f.lists = map[string][]int{"private": {201}}
	for id := 1; id <= 102; id++ {
		f.lists["public"] = append(f.lists["public"], id)
	}
	f.fail, f.failOffset, f.failScope = true, 100, "public"
	store, err := vectorindex.Open(t.Context(), t.TempDir())
	require.NoError(t, err)
	defer store.Close()
	options.Range = &vectorindex.Range{Tail: 2}
	report, err := store.Sync(t.Context(), options)
	require.Error(t, err)
	require.Equal(t, 1, report.Index.Artworks)
	require.Equal(t, vectorindex.RangeComplete, report.Range.Scans["private"].Phase)
	f.fail, f.calls = false, nil
	report, err = store.Sync(t.Context(), options)
	require.NoError(t, err)
	require.Equal(t, []string{"public:100", "public:101"}, f.calls)
	require.Equal(t, 3, report.Index.Artworks)
}

func TestRangeFiltersPreserveUnselectedPendingAndFullScanState(t *testing.T) {
	f, options := newBookmarkFixture(t)
	f.lists, f.invisible = map[string][]int{"public": {1, 2, 3, 4, 5}}, 2
	root := t.TempDir()
	store, err := vectorindex.Open(t.Context(), root)
	require.NoError(t, err)
	defer store.Close()
	db, err := sql.Open("sqlite3", filepath.Join(vectorindex.Directory(root), "index.db"))
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`INSERT INTO artwork VALUES('99','{"type":"artwork","id":"99"}','2026-01-01T00:00:00Z'); INSERT INTO page VALUES('99',0,'https://i.pximg.net/99.png',''); INSERT INTO bookmark VALUES(22,'private','99'); INSERT INTO state VALUES('bookmarks/11/public','{"complete":true}');`)
	require.NoError(t, err)
	options.Range, options.Restrict = &vectorindex.Range{Limit: 3}, "public"
	options.Filter, err = searchfilter.NormalizeFilter("all", "illust")
	require.NoError(t, err)
	report, err := store.Sync(t.Context(), options)
	require.NoError(t, err)
	require.Equal(t, 3, report.Range.Scans["public"].Selected)
	require.Equal(t, 1, report.Range.Scans["public"].Matched)
	require.Equal(t, 1, report.Bookmarks.Unavailable)
	require.Equal(t, 1, report.Embedded)
	require.Equal(t, 2, report.Index.Pending)
	var state string
	require.NoError(t, db.QueryRow(`SELECT value FROM state WHERE key='bookmarks/11/public'`).Scan(&state))
	require.Equal(t, `{"complete":true}`, state)
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM bookmark WHERE account_id=22`).Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM sync_selection`).Scan(&count))
	require.Zero(t, count)
}
