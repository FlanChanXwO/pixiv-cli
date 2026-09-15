package recommended_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/recommended"
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

func TestRecommendedPreservesInitialOffsetPolicyAndMapsArtwork(t *testing.T) {
	transport := &fakeTransport{body: `{"illusts":[{"id":9,"title":"recommended","user":{"id":10},"create_date":"2024-01-02T03:04:05+00:00"}],"next_url":"https://app-api.pixiv.net/v1/illust/recommended?offset=0"}`}
	result, err := recommended.New(transport).List(context.Background(), recommended.Request{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if transport.path != "/v1/illust/recommended" {
		t.Fatalf("path = %q", transport.path)
	}
	if _, present := transport.query["offset"]; present {
		t.Fatalf("initial query unexpectedly contains offset: %v", transport.query)
	}
	if len(result.Items) != 1 || result.Items[0].ID != 9 || !result.HasNext {
		t.Fatalf("result = %#v", result)
	}
	if got := result.NextParams.Get("offset"); got != "0" {
		t.Fatalf("next offset = %q, want \"0\"", got)
	}
}

func TestRecommendedSendsZeroOffsetForContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"illusts":[]}`}
	_, err := recommended.New(transport).List(context.Background(), recommended.Request{ContinuationParams: url.Values{"offset": {"0"}}})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if transport.query.Get("offset") != "0" {
		t.Fatalf("query = %v", transport.query)
	}
}

func TestRecommendedRejectsMissingList(t *testing.T) {
	_, err := recommended.New(&fakeTransport{body: `{"next_url":null}`}).List(context.Background(), recommended.Request{})
	if err == nil {
		t.Fatal("missing artwork list unexpectedly succeeded")
	}
}

func TestRecommendedRejectsInvalidRequestBeforeTransport(t *testing.T) {
	tests := []struct {
		name    string
		request recommended.Request
	}{
		{name: "candidate content type", request: recommended.Request{ContentType: "manga"}},
		{name: "empty continuation params", request: recommended.Request{ContinuationParams: url.Values{}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &fakeTransport{body: `{"illusts":[]}`}
			if _, err := recommended.New(transport).List(context.Background(), test.request); err == nil {
				t.Fatal("invalid request unexpectedly succeeded")
			}
			if transport.calls != 0 {
				t.Fatalf("transport calls = %d, want 0", transport.calls)
			}
		})
	}
}

// G1-CORR-G1-T28-RECOMMENDED-01：live 证据显示 recommended 的 next_url 续页
// 参数是多参数集（min/max_bookmark_id、offset=0、viewed[]、include flags）；
// continuation 必须接受并完整回放该参数集，而不是只允许单一 offset。
func TestRecommendedAcceptsLiveMultiParamContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"illusts":[{"id":9,"title":"recommended","user":{"id":10},"create_date":"2024-01-02T03:04:05+00:00"}],"next_url":"https://app-api.pixiv.net/v1/illust/recommended?min_bookmark_id_for_recent_illust=38760619879&max_bookmark_id_for_recommend=38712263830&offset=0&include_ranking_illusts=false&include_privacy_policy=false&viewed%5B0%5D=144455829&viewed%5B1%5D=149255689"}`}
	result, err := recommended.New(transport).List(context.Background(), recommended.Request{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !result.HasNext {
		t.Fatal("multi-param next_url must produce a continuation")
	}
	// viewed[] 是上游会话参数，live 证明回放会被 400 拒绝，提取时必须剔除。
	if _, ok := result.NextParams["viewed[0]"]; ok {
		t.Fatalf("viewed params must be dropped from continuation: %v", result.NextParams)
	}
	if result.NextParams.Get("min_bookmark_id_for_recent_illust") != "38760619879" {
		t.Fatalf("continuation params = %v", result.NextParams)
	}
}
