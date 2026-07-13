package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckedWebPaginationUsesMachineArithmeticBoundaries(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, pageSize := range []int{60, 50} {
		t.Run(fmt.Sprintf("page size %d", pageSize), func(t *testing.T) {
			largestSafe := (maxInt/pageSize)*pageSize - 1
			pagination, err := checkedWebPagination(largestSafe, pageSize)
			require.NoError(t, err)
			assert.Equal(t, largestSafe/pageSize+1, pagination.page)
			assert.Equal(t, (maxInt/pageSize)*pageSize, pagination.nextOffset)
			assert.Greater(t, pagination.nextOffset, largestSafe)

			_, err = checkedWebPagination(largestSafe+1, pageSize)
			assert.True(t, errors.Is(err, ErrUnrepresentablePagination), "error = %v", err)

			pageStart := largestSafe - largestSafe%pageSize
			remaining := maxInt - pageStart
			assert.True(t, webHasNext(largestSafe, remaining-1, int64(maxInt), pageSize))
			assert.False(t, webHasNext(largestSafe, remaining, int64(maxInt), pageSize))
		})
	}
}

func TestWebHasNextKeepsTotalAsInt64(t *testing.T) {
	const aboveMaxInt32 = int64(1<<31) + 1
	assert.True(t, webHasNext(1, 60, aboveMaxInt32, 60))
	assert.False(t, webHasNext(120, 1, int64(120), 60))
	assert.True(t, webHasNext(61, 1, int64(62), 60))
}

func TestClientContinuationPreservesTotalsBeyondInt32(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		call       func(*Client) (int, bool, error)
		wantOffset int
	}{
		{
			name: "search illust",
			body: `{"error":false,"body":{"illustManga":{"total":2147483648,"data":[{"id":"1","userId":"10"}]}}}`,
			call: func(client *Client) (int, bool, error) {
				result, err := client.SearchIllust(context.Background(), "miku", "partial_match_for_tags", "date_desc", "", 0)
				if err != nil {
					return 0, false, err
				}
				return result.NextOffset, result.ContinuationExists, nil
			},
			wantOffset: 60,
		},
		{
			name: "illust ranking",
			body: `{"rank_total":2147483648,"contents":[{"illust_id":1,"user_id":10}]}`,
			call: func(client *Client) (int, bool, error) {
				result, err := client.IllustRanking(context.Background(), "day", "", 0)
				if err != nil {
					return 0, false, err
				}
				return result.NextOffset, result.ContinuationExists, nil
			},
			wantOffset: 50,
		},
		{
			name: "search user",
			body: `{"error":false,"body":{"illustManga":{"total":2147483648,"data":[{"id":"1","userId":"10"}]}}}`,
			call: func(client *Client) (int, bool, error) {
				result, err := client.SearchUser(context.Background(), "artist", 0)
				if err != nil {
					return 0, false, err
				}
				return result.NextOffset, result.ContinuationExists, nil
			},
			wantOffset: 60,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			client := New(WithHTTPClient(server.Client()), WithWebBase(server.URL))
			offset, continuation, err := test.call(client)
			require.NoError(t, err)
			assert.True(t, continuation)
			assert.Equal(t, test.wantOffset, offset)
		})
	}
}

func TestWebContinuationUsesPageStartForInPageOffsets(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		offset    int
		pageSize  int
		total     int
		wantNext  bool
	}{
		{name: "search total below page boundary", operation: "search", offset: 1, pageSize: 60, total: 59},
		{name: "search total at page boundary", operation: "search", offset: 1, pageSize: 60, total: 60},
		{name: "search total beyond page boundary", operation: "search", offset: 1, pageSize: 60, total: 61, wantNext: true},
		{name: "search without total keeps full-batch continuation", operation: "search", offset: 1, pageSize: 60, wantNext: true},
		{name: "search user total below page boundary", operation: "user", offset: 1, pageSize: 60, total: 59},
		{name: "search user total at page boundary", operation: "user", offset: 1, pageSize: 60, total: 60},
		{name: "search user total beyond page boundary", operation: "user", offset: 1, pageSize: 60, total: 61, wantNext: true},
		{name: "ranking total below page boundary", operation: "ranking", offset: 30, pageSize: 50, total: 49},
		{name: "ranking total at page boundary", operation: "ranking", offset: 30, pageSize: 50, total: 50},
		{name: "ranking total beyond page boundary", operation: "ranking", offset: 30, pageSize: 50, total: 51, wantNext: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				items := make([]map[string]any, test.pageSize)
				for index := range items {
					items[index] = map[string]any{"id": index + 1, "illust_id": index + 1, "userId": index + 101, "user_id": index + 101}
				}
				if test.operation == "ranking" {
					_ = json.NewEncoder(w).Encode(map[string]any{"contents": items, "rank_total": test.total})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"error": false, "body": map[string]any{"illustManga": map[string]any{"data": items, "total": test.total}}})
			}))
			defer server.Close()
			client := New(WithHTTPClient(server.Client()), WithWebBase(server.URL))
			var gotNext bool
			switch test.operation {
			case "ranking":
				result, err := client.IllustRanking(context.Background(), "day", "", test.offset)
				require.NoError(t, err)
				gotNext = result.ContinuationExists
			case "user":
				result, err := client.SearchUser(context.Background(), "artist", test.offset)
				require.NoError(t, err)
				gotNext = result.ContinuationExists
			default:
				result, err := client.SearchIllust(context.Background(), "miku", "partial_match_for_tags", "date_desc", "", test.offset)
				require.NoError(t, err)
				gotNext = result.ContinuationExists
			}
			assert.Equal(t, test.wantNext, gotNext)
		})
	}
}

func TestClientSearchIllustMapsWebArtworkResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/ajax/search/artworks/初音ミク", r.URL.Path)
		assert.Equal(t, "初音ミク", r.URL.Query().Get("word"))
		assert.Equal(t, "date_d", r.URL.Query().Get("order"))
		assert.Equal(t, "s_tag", r.URL.Query().Get("s_mode"))
		assert.Equal(t, "1", r.URL.Query().Get("p"))
		fmt.Fprint(w, `{"error":false,"message":"","body":{"illustManga":{"data":[{"id":"123","title":"Miku","illustType":0,"xRestrict":0,"aiType":1,"url":"https://i.pximg.net/thumb.jpg","tags":["初音ミク"],"userId":"456","userName":"artist","pageCount":1}]}}}`)
	}))
	defer server.Close()

	client := New(WithHTTPClient(server.Client()), WithWebBase(server.URL))
	result, err := client.SearchIllust(context.Background(), "初音ミク", "partial_match_for_tags", "date_desc", "", 0)

	require.NoError(t, err)
	require.Len(t, result.Illusts, 1)
	illust := result.Illusts[0]
	assert.Equal(t, int64(123), illust.ID)
	assert.Equal(t, "Miku", illust.Title)
	assert.Equal(t, "illust", illust.Type)
	assert.Equal(t, 1, illust.AIType)
	assert.Equal(t, int64(456), illust.User.ID)
	assert.Equal(t, "artist", illust.User.Name)
	assert.Equal(t, "https://i.pximg.net/thumb.jpg", illust.ImageURLs.Medium)
	require.Len(t, illust.Tags, 1)
	assert.Equal(t, "初音ミク", illust.Tags[0].Name)
}

func TestClientSearchIllustAppliesInPageOffset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "1", r.URL.Query().Get("p"))
		fmt.Fprint(w, `{"error":false,"message":"","body":{"illustManga":{"data":[{"id":"100","title":"Skip","illustType":0,"userId":"1","userName":"artist","pageCount":1},{"id":"101","title":"Keep","illustType":0,"userId":"1","userName":"artist","pageCount":1}]}}}`)
	}))
	defer server.Close()

	client := New(WithHTTPClient(server.Client()), WithWebBase(server.URL))
	result, err := client.SearchIllust(context.Background(), "初音ミク", "partial_match_for_tags", "date_desc", "", 1)

	require.NoError(t, err)
	require.Len(t, result.Illusts, 1)
	assert.Equal(t, int64(101), result.Illusts[0].ID)
}

func TestClientIllustDetailMapsDetailAndPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ajax/illust/123":
			fmt.Fprint(w, `{"error":false,"message":"","body":{"id":"123","illustId":"123","title":"Miku","illustType":1,"xRestrict":1,"pageCount":2,"userId":"456","userName":"artist","bookmarkCount":12,"viewCount":34,"tags":{"tags":[{"tag":"初音ミク","translation":{"en":"Hatsune Miku"}}]},"urls":{"small":"https://i.pximg.net/small.jpg","regular":"https://i.pximg.net/regular.jpg"}}}`)
		case "/ajax/illust/123/pages":
			fmt.Fprint(w, `{"error":false,"message":"","body":[{"urls":{"thumb_mini":"https://i.pximg.net/t0.jpg","small":"https://i.pximg.net/s0.jpg","regular":"https://i.pximg.net/r0.jpg","original":"https://i.pximg.net/o0.jpg"}},{"urls":{"thumb_mini":"https://i.pximg.net/t1.jpg","small":"https://i.pximg.net/s1.jpg","regular":"https://i.pximg.net/r1.jpg","original":"https://i.pximg.net/o1.jpg"}}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(WithHTTPClient(server.Client()), WithWebBase(server.URL))
	result, err := client.IllustDetail(context.Background(), 123)

	require.NoError(t, err)
	illust := result.Illust
	assert.Equal(t, int64(123), illust.ID)
	assert.Equal(t, "manga", illust.Type)
	assert.Equal(t, 2, illust.PageCount)
	assert.Equal(t, 12, illust.TotalBookmarks)
	assert.Equal(t, 34, illust.TotalView)
	assert.Equal(t, 1, illust.XRestrict)
	require.Len(t, illust.MetaPages, 2)
	assert.Equal(t, "https://i.pximg.net/o0.jpg", illust.MetaPages[0].ImageURLs.Original)
	require.Len(t, illust.Tags, 1)
	assert.Equal(t, "Hatsune Miku", illust.Tags[0].TranslatedName)
}

func TestClientIllustPagesMapsDimensionsIndexAndQuerySafeExtension(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/ajax/illust/123/pages", r.URL.Path)
		fmt.Fprint(w, `{"error":false,"message":"","body":[{"width":1200,"height":1600,"urls":{"original":"https://i.pximg.net/o0.png?token=a.jpg#fragment"}},{"width":2400,"height":1800,"urls":{"original":"https://i.pximg.net/o1.jpeg"}}]}`)
	}))
	defer server.Close()

	client := New(WithHTTPClient(server.Client()), WithWebBase(server.URL))
	pages, err := client.IllustPages(context.Background(), 123)

	require.NoError(t, err)
	require.Len(t, pages, 2)
	assert.Equal(t, 0, pages[0].PageIndex)
	assert.Equal(t, 1200, pages[0].Width)
	assert.Equal(t, 1600, pages[0].Height)
	assert.Equal(t, "png", pages[0].Extension)
	assert.Equal(t, 1, pages[1].PageIndex)
	assert.Equal(t, 2400, pages[1].Width)
	assert.Equal(t, 1800, pages[1].Height)
	assert.Equal(t, "jpeg", pages[1].Extension)
}

func TestClientRankingMapsRankingContents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/ranking.php", r.URL.Path)
		assert.Equal(t, "json", r.URL.Query().Get("format"))
		assert.Equal(t, "daily", r.URL.Query().Get("mode"))
		assert.Equal(t, "1", r.URL.Query().Get("p"))
		fmt.Fprint(w, `{"contents":[{"title":"Ranked","tags":["創作"],"url":"https://i.pximg.net/ranked.jpg","illust_type":"0","illust_page_count":"1","user_name":"artist","illust_id":789,"user_id":456,"rating_count":98,"view_count":765}]}`)
	}))
	defer server.Close()

	client := New(WithHTTPClient(server.Client()), WithWebBase(server.URL))
	result, err := client.IllustRanking(context.Background(), "day", "", 0)

	require.NoError(t, err)
	require.Len(t, result.Illusts, 1)
	assert.Equal(t, int64(789), result.Illusts[0].ID)
	assert.Equal(t, "Ranked", result.Illusts[0].Title)
	assert.Equal(t, 98, result.Illusts[0].TotalBookmarks)
	assert.Equal(t, 765, result.Illusts[0].TotalView)
}

func TestClientRankingAppliesInPageOffset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "1", r.URL.Query().Get("p"))
		fmt.Fprint(w, `{"contents":[{"title":"Skip","illust_type":"0","illust_page_count":"1","user_name":"artist","illust_id":100,"user_id":456},{"title":"Keep","illust_type":"0","illust_page_count":"1","user_name":"artist","illust_id":101,"user_id":456}]}`)
	}))
	defer server.Close()

	client := New(WithHTTPClient(server.Client()), WithWebBase(server.URL))
	result, err := client.IllustRanking(context.Background(), "day", "", 1)

	require.NoError(t, err)
	require.Len(t, result.Illusts, 1)
	assert.Equal(t, int64(101), result.Illusts[0].ID)
}

func TestClientSearchUserAggregatesArtworkAuthors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"error":false,"message":"","body":{"illustManga":{"data":[{"id":"1","title":"A","illustType":0,"userId":"456","userName":"artist","pageCount":1},{"id":"2","title":"B","illustType":0,"userId":"456","userName":"artist","pageCount":1},{"id":"3","title":"C","illustType":0,"userId":"789","userName":"other","pageCount":1}]}}}`)
	}))
	defer server.Close()

	client := New(WithHTTPClient(server.Client()), WithWebBase(server.URL))
	result, err := client.SearchUser(context.Background(), "初音ミク", 0)

	require.NoError(t, err)
	require.Len(t, result.UserPreviews, 2)
	assert.Equal(t, int64(456), result.UserPreviews[0].User.ID)
	assert.Equal(t, int64(789), result.UserPreviews[1].User.ID)
}

func TestClientUgoiraMetadataUsesWebShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ajax/illust/123/ugoira_meta":
			fmt.Fprint(w, `{"error":false,"message":"","body":{"src":"https://i.pximg.net/ugoira-medium.zip","originalSrc":"https://i.pximg.net/ugoira.zip","frames":[{"file":"000000.jpg","delay":80}]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(WithHTTPClient(server.Client()), WithWebBase(server.URL))
	meta, err := client.UgoiraMetadata(context.Background(), 123)
	require.NoError(t, err)
	assert.Equal(t, "https://i.pximg.net/ugoira-medium.zip", meta.UgoiraMetadata.ZipURLs.Medium)
	assert.Equal(t, "https://i.pximg.net/ugoira.zip", meta.UgoiraMetadata.ZipURLs.Original)
	require.Len(t, meta.UgoiraMetadata.Frames, 1)
	assert.Equal(t, 80, meta.UgoiraMetadata.Frames[0].Delay)
}

func TestClientExposesWebHTTPErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "blocked", http.StatusForbidden)
	}))
	defer server.Close()

	client := New(WithHTTPClient(server.Client()), WithWebBase(server.URL))
	_, err := client.IllustRanking(context.Background(), "day", "", 0)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pixiv web api error: status 403")
	assert.Contains(t, err.Error(), "blocked")
}
