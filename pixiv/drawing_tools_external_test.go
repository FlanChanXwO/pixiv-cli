package pixiv_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestSupportedDrawingToolsReturnsOrderedDefensiveCatalog(t *testing.T) {
	payload, err := os.ReadFile("drawing-tools.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(payload)); got != "eb2415c94571ed7ca9ccfd60e9d97e5d0505f23d75535d3b7f1402a348c90f2c" {
		t.Fatalf("drawing-tool catalog digest=%s", got)
	}
	var want []string
	if err := json.Unmarshal(payload, &want); err != nil {
		t.Fatal(err)
	}
	if len(want) != 101 {
		t.Fatalf("drawing-tool catalog length=%d, want 101", len(want))
	}
	tools := pixiv.SupportedDrawingTools()
	if !slices.Equal(tools, want) {
		t.Fatalf("SupportedDrawingTools does not preserve the JSON catalog order")
	}
	tools[0] = "changed"
	if pixiv.SupportedDrawingTools()[0] != "SAI" {
		t.Fatal("SupportedDrawingTools returned the mutable catalog backing array")
	}
}

func TestSearchIllustValidatesDrawingToolAndSuggestsOnlyUnambiguousCorrection(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"illusts":[]}`))
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.NewClientOptions{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		tool string
		want string
	}{
		{tool: "Photoshp", want: `did you mean "Photoshop"`},
		{tool: "pho", want: "drawing-tool-catalog"},
		{tool: "not-a-tool", want: "drawing-tool-catalog"},
	} {
		_, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku", Filters: pixiv.SearchIllustFilters{Tool: test.tool}})
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("tool=%q error=%v, want %q", test.tool, err, test.want)
		}
		if test.tool == "pho" && strings.Contains(err.Error(), "did you mean") {
			t.Fatalf("ambiguous prefix received a suggestion: %v", err)
		}
	}
	if calls != 0 {
		t.Fatalf("drawing tool validation made %d network calls", calls)
	}
}
