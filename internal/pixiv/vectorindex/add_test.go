package vectorindex_test

import (
	"context"
	"database/sql"
	"errors"
	"iter"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/searchfilter"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/vectorindex"
	"github.com/stretchr/testify/require"
)

func TestAddDeduplicatesReusesAndStopsAtMissingArtwork(t *testing.T) {
	_, options := newBookmarkFixture(t)
	store, err := vectorindex.Open(t.Context(), t.TempDir())
	require.NoError(t, err)
	defer store.Close()
	ids := func(yield func(int64, error) bool) {
		for _, id := range []int64{23, 23, 24, 404, 25} {
			if !yield(id, nil) {
				return
			}
		}
	}
	report, err := store.Add(t.Context(), ids, options)
	require.Error(t, err)
	require.Equal(t, 2, report.Recorded)
	require.Equal(t, 2, report.Embedded)
	require.Equal(t, 2, report.Index.Artworks)
	require.Equal(t, 0, report.Index.Pending)
	report, err = store.Add(t.Context(), func(yield func(int64, error) bool) { yield(23, nil) }, options)
	require.NoError(t, err)
	require.Equal(t, 0, report.Embedded)
	require.Equal(t, 2, report.Index.Pages)
	sentinel := errors.New("input failed")
	report, err = store.Add(t.Context(), func(yield func(int64, error) bool) {
		if yield(26, nil) {
			yield(0, sentinel)
		}
	}, options)
	require.ErrorIs(t, err, sentinel)
	require.Equal(t, 3, report.Index.Artworks)
}

func TestSyncFiltersAndLaterUnfilteredCatchUp(t *testing.T) {
	f, options := newBookmarkFixture(t)
	f.lists = map[string][]int{"public": {1, 2}, "private": {3}}
	for _, tc := range []struct {
		scope, rating, content string
		pages                  int
	}{
		{"all", "all", "all", 3}, {"public", "all", "all", 2},
		{"private", "all", "all", 1}, {"all", "all", "illust", 1},
		{"all", "sfw", "all", 1}, {"public", "r18", "manga", 1},
		{"private", "mature", "ugoira", 1}, {"private", "all", "illust", 0},
	} {
		t.Run(tc.scope+tc.rating+tc.content, func(t *testing.T) {
			store, err := vectorindex.Open(t.Context(), t.TempDir())
			require.NoError(t, err)
			defer store.Close()
			options.Restrict = tc.scope
			options.Filter, err = searchfilter.NormalizeFilter(tc.rating, tc.content)
			require.NoError(t, err)
			report, err := store.Sync(t.Context(), options)
			require.NoError(t, err)
			require.Equal(t, tc.pages, report.Index.Pages)
			options.Restrict, options.Filter = "all", searchfilter.Filter{}
			report, err = store.Sync(t.Context(), options)
			require.NoError(t, err)
			require.Equal(t, 3, report.Index.Pages)
			require.Zero(t, report.Index.Pending)
		})
	}
}

func TestAddCanceledInputPreservesCompletedResult(t *testing.T) {
	_, options := newBookmarkFixture(t)
	store, err := vectorindex.Open(t.Context(), t.TempDir())
	require.NoError(t, err)
	defer store.Close()
	ctx, cancel := context.WithCancel(t.Context())
	ids := iter.Seq2[int64, error](func(yield func(int64, error) bool) {
		if yield(23, nil) {
			cancel()
			yield(24, nil)
		}
	})
	report, err := store.Add(ctx, ids, options)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, report.Index.Artworks)
	require.Equal(t, 1, report.Index.Pages)
}

func TestHeadDoesNotStopAtIncompleteReconcileMembership(t *testing.T) {
	f, options := newBookmarkFixture(t)
	store, err := vectorindex.Open(t.Context(), t.TempDir())
	require.NoError(t, err)
	defer store.Close()
	options.Restrict = "public"
	f.lists = map[string][]int{"public": {1}}
	_, err = store.Sync(t.Context(), options)
	require.NoError(t, err)
	f.lists["public"], f.fail, f.failOffset = []int{3, 2, 1}, true, 1
	options.Reconcile = true
	_, err = store.Sync(t.Context(), options)
	require.Error(t, err)
	f.fail, options.Reconcile = false, false
	report, err := store.Sync(t.Context(), options)
	require.NoError(t, err)
	require.Equal(t, 3, report.Index.Artworks)
}

func TestReconcileKeepsReturnedUnavailableMembership(t *testing.T) {
	f, options := newBookmarkFixture(t)
	store, err := vectorindex.Open(t.Context(), t.TempDir())
	require.NoError(t, err)
	defer store.Close()
	options.Restrict = "public"
	f.lists = map[string][]int{"public": {1, 2}}
	_, err = store.Sync(t.Context(), options)
	require.NoError(t, err)
	f.invisible, options.Reconcile = 1, true
	report, err := store.Sync(t.Context(), options)
	require.NoError(t, err)
	require.Equal(t, 1, report.Bookmarks.Unavailable)
	require.Zero(t, report.Bookmarks.Removed)
	options.Reconcile = false
	report, err = store.Sync(t.Context(), options)
	require.NoError(t, err)
	require.Equal(t, 1, report.Bookmarks.Pages)
}

func TestInvalidHeadCursorRetainsUnconfirmedPrefix(t *testing.T) {
	f, options := newBookmarkFixture(t)
	root := t.TempDir()
	store, err := vectorindex.Open(t.Context(), root)
	require.NoError(t, err)
	defer store.Close()
	options.Restrict = "public"
	f.lists = map[string][]int{"public": {1}}
	_, err = store.Sync(t.Context(), options)
	require.NoError(t, err)
	f.lists["public"], f.fail, f.failOffset = []int{3, 2, 1}, true, 1
	_, err = store.Sync(t.Context(), options)
	require.Error(t, err)
	db, err := sql.Open("sqlite3", filepath.Join(vectorindex.Directory(root), "index.db"))
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE state SET value='{"cursor":"!invalid!","complete":false}' WHERE key='bookmarks/11/public/head'`)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	f.fail = false
	report, err := store.Sync(t.Context(), options)
	require.NoError(t, err)
	require.Equal(t, 3, report.Index.Artworks)
}
