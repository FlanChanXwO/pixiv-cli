package pixiv_test

import (
	"encoding/json"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestIllustJSONPutsURLFirstAndUsesArtworkPage(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(pixiv.Illust{URL: "https://www.pixiv.net/artworks/42", ID: 42, Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if text[:len(`{"url":`)] != `{"url":` {
		t.Fatalf("json field order = %s, want url first", text)
	}
	if !json.Valid(raw) {
		t.Fatal("invalid json")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	var url string
	if err := json.Unmarshal(obj["url"], &url); err != nil || url != "https://www.pixiv.net/artworks/42" {
		t.Fatalf("url=%q err=%v", url, err)
	}
}
