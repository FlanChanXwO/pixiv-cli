package pixiv_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

// exclude AI：请求必须带 search_ai_type=1，且在 canary 证明前仍本地后筛 AIType==2。
func TestSearchIllustExcludeAISendsBackendParamAndLocalPostFilters(t *testing.T) {
	t.Parallel()
	var sawSearchAIType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawSearchAIType = r.URL.Query().Get("search_ai_type")
		// 故意让上游仍返回 AI 作品，模拟后端未完全生效的情况。
		fmt.Fprint(w, `{"illusts":[
			{"id":1,"title":"ai","type":"illust","page_count":1,"x_restrict":0,"illust_ai_type":2},
			{"id":2,"title":"human","type":"illust","page_count":1,"x_restrict":0,"illust_ai_type":0},
			{"id":3,"title":"legacy-ai","type":"illust","page_count":1,"x_restrict":0,"ai_type":2},
			{"id":4,"title":"legacy-human","type":"illust","page_count":1,"x_restrict":0,"ai_type":1}
		]}`)
	}))
	defer server.Close()

	client, err := pixiv.NewClient(pixiv.NewClientOptions{
		HTTPClient:    server.Client(),
		AppAPIBaseURL: server.URL,
		AccessToken:   "token",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{
		Word:    "miku",
		Filters: pixiv.SearchIllustFilters{AIMode: pixiv.SearchAIModeExclude},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sawSearchAIType != "1" {
		t.Fatalf("search_ai_type = %q, want 1", sawSearchAIType)
	}
	if len(result.Illusts) != 2 || result.Illusts[0].ID != 2 || result.Illusts[1].ID != 4 {
		t.Fatalf("exclude AI illusts = %#v, want ids 2,4", result.Illusts)
	}
	for _, illust := range result.Illusts {
		if illust.AIType == 2 {
			t.Fatalf("local post-filter leaked AIType==2: %#v", illust)
		}
	}
}

// tool/type/ratio/resolution 只走 App query，不做本地重复过滤：
// 上游若仍返回“不符合参数”的作品，public SDK 本批次仍原样保留。
func TestSearchIllustBackendOnlyFiltersDoNotLocallyRefilterBatch(t *testing.T) {
	t.Parallel()
	var gotQuery map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		gotQuery = map[string]string{
			"content_type":   q.Get("content_type"),
			"ratio_pattern":  q.Get("ratio_pattern"),
			"width_min":      q.Get("width_min"),
			"height_min":     q.Get("height_min"),
			"tool":           q.Get("tool"),
			"search_ai_type": q.Get("search_ai_type"),
		}
		// 上游返回与参数明显不符的作品（manga 请求却给 ugoira、错误 tool、小尺寸）。
		fmt.Fprint(w, `{"illusts":[
			{"id":11,"title":"wrong-type","type":"ugoira","page_count":1,"x_restrict":0,"width":100,"height":100,"tools":["Other"],"ai_type":0},
			{"id":12,"title":"ok-looking","type":"manga","page_count":2,"x_restrict":0,"width":4000,"height":4000,"tools":["CLIP STUDIO PAINT"],"ai_type":0}
		]}`)
	}))
	defer server.Close()

	client, err := pixiv.NewClient(pixiv.NewClientOptions{
		HTTPClient:    server.Client(),
		AppAPIBaseURL: server.URL,
		AccessToken:   "token",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{
		Word: "miku",
		Filters: pixiv.SearchIllustFilters{
			ContentType: pixiv.SearchContentTypeManga,
			AspectRatio: pixiv.SearchAspectRatioPortrait,
			Resolution:  pixiv.SearchResolutionHigh,
			Tool:        "CLIP STUDIO PAINT",
			AIMode:      pixiv.SearchAIModeAll,
			Rating:      pixiv.SearchRatingAll,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantQuery := map[string]string{
		"content_type":   "manga",
		"ratio_pattern":  "portrait",
		"width_min":      "3000",
		"height_min":     "3000",
		"tool":           "CLIP STUDIO PAINT",
		"search_ai_type": "0",
	}
	for key, want := range wantQuery {
		if gotQuery[key] != want {
			t.Fatalf("query[%s]=%q want %q; full=%v", key, gotQuery[key], want, gotQuery)
		}
	}
	// 关键：不得因 type/tool/尺寸本地丢弃 id=11。
	if len(result.Illusts) != 2 || result.Illusts[0].ID != 11 || result.Illusts[1].ID != 12 {
		t.Fatalf("backend-only filters must not refilter batch: %#v", result.Illusts)
	}
}

// rating 继续只按响应 x_restrict 本地筛选；App 无可靠分级 query。
func TestSearchIllustRatingFiltersLocallyByXRestrictOnly(t *testing.T) {
	t.Parallel()
	var hadRestrictQuery bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for key := range r.URL.Query() {
			if key == "x_restrict" || key == "mode" && r.URL.Query().Get(key) == "r18" {
				hadRestrictQuery = true
			}
		}
		fmt.Fprint(w, `{"illusts":[
			{"id":1,"x_restrict":0,"ai_type":0},
			{"id":2,"x_restrict":1,"ai_type":0},
			{"id":3,"x_restrict":2,"ai_type":0}
		]}`)
	}))
	defer server.Close()

	client, err := pixiv.NewClient(pixiv.NewClientOptions{
		HTTPClient:    server.Client(),
		AppAPIBaseURL: server.URL,
		AccessToken:   "token",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{
		Word:    "miku",
		Filters: pixiv.SearchIllustFilters{Rating: pixiv.SearchRatingR18},
	})
	if err != nil {
		t.Fatal(err)
	}
	if hadRestrictQuery {
		t.Fatal("rating must not invent unsupported App restrict query params")
	}
	if len(result.Illusts) != 1 || result.Illusts[0].ID != 2 || result.Illusts[0].XRestrict != 1 {
		t.Fatalf("r18 local filter = %#v", result.Illusts)
	}
}
