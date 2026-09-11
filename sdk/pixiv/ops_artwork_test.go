package pixiv_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestRecommendedArtworksPreservesExplicitZeroContinuation(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v1/illust/recommended" {
			t.Errorf("path = %q", req.URL.Path)
		}
		query := req.URL.Query()
		if calls == 1 {
			if _, ok := query["offset"]; ok {
				t.Errorf("initial request must not include offset: %v", query)
			}
		} else if query.Get("offset") != "0" {
			t.Errorf("continuation offset = %q, want %q", query.Get("offset"), "0")
		}
		body := `{"illusts":[{"id":7301,"title":"recommended artwork","type":"illust","create_date":"2026-01-05T00:00:00Z","user":{"id":21,"name":"artist"}}],"next_url":"https://app-api.pixiv.net/v1/illust/recommended?offset=0"}`
		if calls == 2 {
			body = `{"illusts":[],"next_url":null}`
		}
		return jsonResponse(body), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	request := RecommendedArtworksRequest{}
	page, err := client.RecommendedArtworks(context.Background(), request)
	if err != nil {
		t.Fatalf("RecommendedArtworks: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 7301 || page.Next.IsZero() {
		t.Fatalf("first page = %#v", page)
	}
	request.Cursor = page.Next
	page, err = client.RecommendedArtworks(context.Background(), request)
	if err != nil {
		t.Fatalf("RecommendedArtworks continuation: %v", err)
	}
	if page.Items == nil || len(page.Items) != 0 || !page.Next.IsZero() || calls != 2 {
		t.Fatalf("second page = %#v calls=%d", page, calls)
	}
}

// G1-CORR-G1-T28-RECOMMENDED-01：live next_url 的多参数续页集必须被完整
// 回放；cursor 文本不得泄漏 raw next_url/凭据，binding v2 使旧单 offset
// cursor 显式失效。
func TestRecommendedArtworksReplaysFullLiveContinuationParams(t *testing.T) {
	calls := 0
	liveNextURL := "https://app-api.pixiv.net/v1/illust/recommended?min_bookmark_id_for_recent_illust=38760619879&max_bookmark_id_for_recommend=38712263830&offset=0&include_ranking_illusts=false&include_privacy_policy=false&viewed%5B0%5D=144455829&viewed%5B1%5D=149255689"
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			if _, ok := req.URL.Query()["min_bookmark_id_for_recent_illust"]; ok {
				t.Errorf("initial request must not include continuation params: %v", req.URL.RawQuery)
			}
		} else {
			query := req.URL.Query()
			// viewed[] 是上游会话参数，回放会被 400 拒绝（G1-T28 live 证据），
			// 因此续页请求必须剔除 viewed、保留其余 live 参数。
			if query.Get("min_bookmark_id_for_recent_illust") != "38760619879" ||
				query.Get("max_bookmark_id_for_recommend") != "38712263830" ||
				query.Get("offset") != "0" {
				t.Errorf("continuation query lost live params: %v", query)
			}
			for key := range query {
				if strings.HasPrefix(key, "viewed[") {
					t.Errorf("continuation query must not replay viewed params: %v", query)
					break
				}
			}
		}
		body := `{"illusts":[{"id":7301,"title":"recommended artwork","type":"illust","create_date":"2026-01-05T00:00:00Z","user":{"id":21,"name":"artist"}}],"next_url":"` + liveNextURL + `"}`
		if calls == 2 {
			body = `{"illusts":[],"next_url":null}`
		}
		return jsonResponse(body), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	page, err := client.RecommendedArtworks(context.Background(), RecommendedArtworksRequest{})
	if err != nil {
		t.Fatalf("RecommendedArtworks: %v", err)
	}
	if page.Next.IsZero() {
		t.Fatal("multi-param next_url must produce a continuation cursor")
	}
	if strings.Contains(page.Next.String(), "http") || strings.Contains(page.Next.String(), "token") {
		t.Fatalf("cursor text leaks transport material: %s", page.Next.String())
	}
	request := RecommendedArtworksRequest{Cursor: page.Next}
	page, err = client.RecommendedArtworks(context.Background(), request)
	if err != nil {
		t.Fatalf("RecommendedArtworks continuation: %v", err)
	}
	if len(page.Items) != 0 || !page.Next.IsZero() || calls != 2 {
		t.Fatalf("second page = %#v calls=%d", page, calls)
	}
}

func TestRecommendedArtworksRejectsLegacyBindingVersionCursor(t *testing.T) {
	// 旧 v1 recommended cursor（单 offset payload）必须显式 InvalidCursor。
	legacy := buildLegacyRecommendedCursor(t, "RecommendedArtworks", "offset", 0)
	client, err := NewWith("token", Options{})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	_, err = client.RecommendedArtworks(context.Background(), RecommendedArtworksRequest{Cursor: legacy})
	if sdk.ReasonOf(err) != sdk.InvalidCursor {
		t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.InvalidCursor, err)
	}
}

// buildLegacyRecommendedCursor 以当前真实 cursor 为模板，回填旧 v1 binding 与
// 单 offset payload，构造升级前遗留 cursor。
func buildLegacyRecommendedCursor(t *testing.T, operation, key string, value int64) sdk.Cursor {
	t.Helper()
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/illust/recommended?offset=0"}`
		return jsonResponse(body), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	page, err := client.RecommendedArtworks(context.Background(), RecommendedArtworksRequest{})
	if err != nil {
		t.Fatalf("mint real cursor: %v", err)
	}
	if page.Next.IsZero() {
		t.Fatal("expected a continuation cursor to derive the legacy envelope from")
	}
	raw, err := base64.RawURLEncoding.DecodeString(page.Next.String())
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	envelope["b"] = 1
	envelope["pl"] = []byte(`{"k":"` + key + `","v":` + strconv.FormatInt(value, 10) + `}`)
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal legacy envelope: %v", err)
	}
	legacy, err := sdk.ParseCursor(base64.RawURLEncoding.EncodeToString(encoded))
	if err != nil {
		t.Fatalf("parse legacy cursor: %v", err)
	}
	return legacy
}
