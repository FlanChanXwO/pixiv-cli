package pixiv_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

// App 响应可能只给 illust_ai_type；public AIType 必须映射正确，且本地 only/exclude 仍以 ==2 为准。
func TestSearchIllustMapsIllustAITypeFromAppWire(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/search/illust" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"illusts":[
			{"id":1,"title":"new-ai","type":"illust","page_count":1,"x_restrict":0,"illust_ai_type":2},
			{"id":2,"title":"legacy-ai","type":"illust","page_count":1,"x_restrict":0,"ai_type":2},
			{"id":3,"title":"prefer-new-non-ai","type":"illust","page_count":1,"x_restrict":0,"illust_ai_type":1,"ai_type":2},
			{"id":4,"title":"human","type":"illust","page_count":1,"x_restrict":0,"illust_ai_type":0}
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

	// only AI：本地筛选固定 AIType==2
	only, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{
		Word:    "miku",
		Filters: pixiv.SearchIllustFilters{AIMode: pixiv.SearchAIModeOnly},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(only.Illusts) != 2 || only.Illusts[0].ID != 1 || only.Illusts[1].ID != 2 {
		t.Fatalf("only AI illusts = %#v, want ids 1,2", only.Illusts)
	}
	if only.Illusts[0].AIType != 2 || only.Illusts[1].AIType != 2 {
		t.Fatalf("only AI types = %d,%d want 2,2", only.Illusts[0].AIType, only.Illusts[1].AIType)
	}

	// all：验证双字段优先级与 0 值
	all, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{
		Word:    "miku",
		Filters: pixiv.SearchIllustFilters{AIMode: pixiv.SearchAIModeAll},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Illusts) != 4 {
		t.Fatalf("all len=%d want 4", len(all.Illusts))
	}
	wantTypes := map[int64]int{1: 2, 2: 2, 3: 1, 4: 0}
	for _, illust := range all.Illusts {
		if got := illust.AIType; got != wantTypes[illust.ID] {
			t.Fatalf("id=%d AIType=%d want %d", illust.ID, got, wantTypes[illust.ID])
		}
	}
}
