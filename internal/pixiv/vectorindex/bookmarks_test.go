package vectorindex_test

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/vectorindex"
	"github.com/FlanChanXwO/pixiv-cli/internal/vectormodel"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

func TestIndexMigrationPreservesProgress(t *testing.T) {
	root := t.TempDir()
	store, err := vectorindex.Open(t.Context(), root)
	require.NoError(t, err)
	require.NoError(t, store.Close())
	db, err := sql.Open("sqlite3", filepath.Join(vectorindex.Directory(root), "index.db"))
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`DROP TABLE IF EXISTS scan_seen; PRAGMA user_version=1;
INSERT INTO state VALUES('bookmarks/11/private','{"complete":true}');
INSERT INTO artwork VALUES('23','{}','2026-11-20T00:00:00Z');
INSERT INTO page VALUES('23',0,'https://i.pximg.net/23.png','');
INSERT INTO embedding VALUES('23',0,'fixture',zeroblob(8),'')`)
	require.NoError(t, err)
	store, err = vectorindex.Open(t.Context(), root)
	require.NoError(t, err)
	defer store.Close()
	var version int
	require.NoError(t, db.QueryRow(`PRAGMA user_version`).Scan(&version))
	require.Equal(t, 2, version)
	counts, err := store.Counts(t.Context(), "fixture")
	require.NoError(t, err)
	require.Equal(t, 1, counts.Pages)
	status, err := vectorindex.ReadStatus(t.Context(), root, "fixture")
	require.NoError(t, err)
	require.Equal(t, `{"complete":true}`, status.Bookmarks["bookmarks/11/private"])
}

func TestBookmarkResumeAndReconcileRetainsSharedArtworks(t *testing.T) {
	f, options := newBookmarkFixture(t)
	root := t.TempDir()
	store, err := vectorindex.Open(t.Context(), root)
	require.NoError(t, err)
	defer store.Close()
	f.lists = map[string][]int{"public": {1, 2, 3}, "private": {4}}
	options.Restrict = "public"
	f.fail, f.failOffset = true, 1
	partial, err := store.Sync(t.Context(), options)
	require.Error(t, err)
	require.Equal(t, "rate_limited", *partial.Stopped)
	f.fail, f.calls = false, nil
	_, err = store.Sync(t.Context(), options)
	require.NoError(t, err)
	require.Equal(t, "public:1", f.calls[0])
	options.Restrict, options.Reconcile = "all", true
	f.lists, f.fail = map[string][]int{"public": {1, 3}, "private": {2, 4}}, true
	_, err = store.Sync(t.Context(), options)
	require.Error(t, err)
	db, err := sql.Open("sqlite3", filepath.Join(vectorindex.Directory(root), "index.db"))
	require.NoError(t, err)
	defer db.Close()
	var n int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM bookmark WHERE restrict='public' AND artwork_id='2'`).Scan(&n))
	require.Equal(t, 1, n)
	f.fail = false
	report, err := store.Sync(t.Context(), options)
	require.NoError(t, err)
	require.Equal(t, 1, report.Bookmarks.Removed)
	require.Equal(t, 4, report.Index.Artworks)
	require.Equal(t, 4, report.Index.Pages)
}

func TestBookmarkHeadResumesAfterInterruption(t *testing.T) {
	f, options := newBookmarkFixture(t)
	store, err := vectorindex.Open(t.Context(), t.TempDir())
	require.NoError(t, err)
	defer store.Close()
	options.Restrict = "public"
	f.lists = map[string][]int{"public": {1, 2}}
	_, err = store.Sync(t.Context(), options)
	require.NoError(t, err)
	f.lists["public"], f.fail, f.failOffset = []int{5, 4, 3, 1, 2}, true, 2
	_, err = store.Sync(t.Context(), options)
	require.Error(t, err)
	f.fail, f.calls = false, nil
	report, err := store.Sync(t.Context(), options)
	require.NoError(t, err)
	require.Equal(t, "public:2", f.calls[0])
	require.Equal(t, "head", report.Bookmarks.Scans["public"].Phase)
	require.True(t, report.Bookmarks.Scans["public"].Complete)
	require.Equal(t, 5, report.Index.Artworks)
}

func TestBookmarkBatchesComputeBeforeContinuing(t *testing.T) {
	f, options := newBookmarkFixture(t)
	store, err := vectorindex.Open(t.Context(), t.TempDir())
	require.NoError(t, err)
	defer store.Close()
	f.lists = map[string][]int{"private": {300}}
	for id := 1; id <= 101; id++ {
		f.lists["public"] = append(f.lists["public"], id)
	}
	report, err := store.Sync(t.Context(), options)
	require.NoError(t, err)
	require.Equal(t, 100, f.firstEmbedding)
	require.Equal(t, 102, report.Bookmarks.Pages)
	require.Equal(t, 102, report.Index.Pages)
	require.True(t, report.Bookmarks.Scans["public"].Complete)
	require.True(t, report.Bookmarks.Scans["private"].Complete)
}

type bookmarkFixture struct {
	lists          map[string][]int
	fail           bool
	failOffset     int
	calls          []string
	firstEmbedding int
	mutex          sync.Mutex
}

func (f *bookmarkFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	switch {
	case strings.Contains(r.URL.Path, "embedContent"):
		if f.firstEmbedding == 0 {
			f.firstEmbedding = len(f.calls)
		}
		fmt.Fprint(w, `{"embedding":{"values":[1,0]}}`)
	case strings.Contains(r.URL.Path, "bookmarks/illust"):
		scope := r.URL.Query().Get("restrict")
		offset, _ := strconv.Atoi(r.URL.Query().Get("max_bookmark_id"))
		f.calls = append(f.calls, fmt.Sprintf("%s:%d", scope, offset))
		if f.fail && offset == f.failOffset {
			w.WriteHeader(429)
			return
		}
		values := f.lists[scope]
		if offset >= len(values) {
			fmt.Fprint(w, `{"illusts":[],"next_url":null}`)
			return
		}
		next := "null"
		if offset+1 < len(values) {
			next = strconv.Quote(fmt.Sprintf("https://app-api.pixiv.net/v1/user/bookmarks/illust?user_id=11&restrict=%s&max_bookmark_id=%d", scope, offset+1))
		}
		fmt.Fprintf(w, `{"illusts":[%s],"next_url":%s}`, fixtureArtwork(values[offset]), next)
	case strings.Contains(r.URL.Path, "illust/detail"):
		id, _ := strconv.Atoi(r.URL.Query().Get("illust_id"))
		if id == 404 {
			w.WriteHeader(404)
			return
		}
		fmt.Fprintf(w, `{"illust":%s}`, fixtureArtwork(id))
	default:
		body, _ := vectormodel.ProbeImage()
		w.Write(body)
	}
}
func fixtureArtwork(id int) string {
	return fmt.Sprintf(`{"id":%d,"type":%q,"x_restrict":%d,"page_count":1,"create_date":"2024-05-01T10:00:00+09:00","user":{"id":7},"meta_single_page":{"original_image_url":"https://i.pximg.net/%d.png"}}`, id, []string{"illust", "manga", "ugoira"}[(id-1)%3], (id-1)%3, id)
}

func newBookmarkFixture(t *testing.T) (*bookmarkFixture, vectorindex.SyncOptions) {
	t.Helper()
	fixture := &bookmarkFixture{}
	server := httptest.NewTLSServer(fixture)
	t.Cleanup(server.Close)
	httpClient := server.Client()
	transport := httpClient.Transport.(*http.Transport)
	transport.TLSClientConfig.ServerName = server.Listener.Addr().(*net.TCPAddr).IP.String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	credentials := pixiv.NewCredentials("fixture-refresh")
	credentials.SetTokens("fixture-access", "fixture-refresh")
	credentials.UserID, credentials.ExpiresAt = 11, time.Now().Add(time.Hour)
	account, err := pixiv.NewMemoryCredentials(credentials)
	require.NoError(t, err)
	client, err := pixiv.New(account, pixiv.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	model, err := vectormodel.Open(vectormodel.Options{Name: "fixture", BaseURL: server.URL, APIKey: "fixture-key"}, httpClient)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, model.Close()) })
	return fixture, vectorindex.SyncOptions{Model: "fixture", Embedder: model, OpenClient: func(context.Context) (int64, *pixiv.Client, func() error, error) {
		return 11, client, func() error { return nil }, nil
	}}
}
