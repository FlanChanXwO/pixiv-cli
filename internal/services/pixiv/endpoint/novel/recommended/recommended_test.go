package recommended_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel/recommended"
)

type fakeTransport struct {
	path  string
	query url.Values
	body  string
	calls int
}

func (f *fakeTransport) GetJSON(_ context.Context, path string, query url.Values, out any) error {
	f.calls++
	f.path = path
	f.query = query
	return json.Unmarshal([]byte(f.body), out)
}

func TestRecommendedRejectsEmptyContinuationParamsBeforeTransport(t *testing.T) {
	transport := &fakeTransport{body: `{"novels":[]}`}
	_, err := recommended.New(transport).List(context.Background(), recommended.Request{ContinuationParams: url.Values{}})
	if err == nil {
		t.Fatal("empty continuation params unexpectedly succeeded")
	}
	if transport.calls != 0 {
		t.Fatalf("transport calls = %d, want 0", transport.calls)
	}
}

func TestRecommendedPreservesInitialAndContinuationOffset(t *testing.T) {
	transport := &fakeTransport{body: `{"novels":[{"id":31,"title":"recommended","user":{"id":7},"create_date":"2024-01-02T03:04:05+00:00"}],"next_url":"https://app-api.pixiv.net/v1/novel/recommended?offset=0"}`}
	client := recommended.New(transport)
	result, err := client.List(context.Background(), recommended.Request{})
	if err != nil {
		t.Fatalf("initial List: %v", err)
	}
	if transport.path != "/v1/novel/recommended" || transport.query.Get("offset") != "" || len(result.Items) != 1 || !result.HasNext {
		t.Fatalf("initial result=%#v request=%q %v", result, transport.path, transport.query)
	}
	if got := result.NextParams.Get("offset"); got != "0" {
		t.Fatalf("next offset = %q, want \"0\"", got)
	}
	result, err = client.List(context.Background(), recommended.Request{ContinuationParams: result.NextParams})
	if err != nil {
		t.Fatalf("continuation List: %v", err)
	}
	if transport.query.Get("offset") != "0" {
		t.Fatalf("continuation query = %v, want offset=0", transport.query)
	}
}

func TestRecommendedRejectsNullNovelList(t *testing.T) {
	_, err := recommended.New(&fakeTransport{body: `{"novels":null}`}).List(context.Background(), recommended.Request{})
	if err == nil {
		t.Fatal("null list unexpectedly succeeded")
	}
}

// G1-CORR-G1-T28-RECOMMENDED-01：live 证据显示 novel recommended 的 next_url
// 续页参数是多参数集（offset、already_recommended、max_bookmark_id_for_recommend、
// include flags）；continuation 必须接受并完整回放该参数集。
func TestRecommendedAcceptsLiveMultiParamContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"novels":[{"id":9,"title":"recommended","user":{"id":10},"create_date":"2024-01-02T03:04:05+00:00"}],"next_url":"https://app-api.pixiv.net/v1/novel/recommended?include_ranking_novels=false&include_privacy_policy=false&offset=15&already_recommended=28977240,29004295&max_bookmark_id_for_recommend=38756871711"}`}
	result, err := recommended.New(transport).List(context.Background(), recommended.Request{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !result.HasNext {
		t.Fatal("multi-param next_url must produce a continuation")
	}
}
