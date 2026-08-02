package sdk_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func TestOpenResourceRequestValidate(t *testing.T) {
	ref, err := sdk.NewResourceRef("pixiv", []byte("artwork:1"))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	if err := (sdk.OpenResourceRequest{Ref: ref}).Validate(); err != nil {
		t.Fatalf("zero method should default to GET and validate: %v", err)
	}
	if err := (sdk.OpenResourceRequest{Ref: ref, Method: sdk.ResourceMethodGet}).Validate(); err != nil {
		t.Fatalf("GET validate: %v", err)
	}
	if err := (sdk.OpenResourceRequest{Ref: ref, Method: sdk.ResourceMethodHead}).Validate(); err != nil {
		t.Fatalf("HEAD validate: %v", err)
	}
	if err := (sdk.OpenResourceRequest{Ref: ref, Method: "POST"}).Validate(); sdk.CodeOf(err) != sdk.CodeInvalidArgument {
		t.Fatalf("POST: expected CodeInvalidArgument, got %v", err)
	}
	if err := (sdk.OpenResourceRequest{Ref: ref, Range: "bytes=0-\x01"}).Validate(); sdk.CodeOf(err) != sdk.CodeInvalidArgument {
		t.Fatalf("control char in range: expected CodeInvalidArgument, got %v", err)
	}
	if err := (sdk.OpenResourceRequest{Ref: ref, IfNoneMatch: "good-etag"}).Validate(); err != nil {
		t.Fatalf("valid etag validate: %v", err)
	}
}

func TestResourceCopyIsDeep(t *testing.T) {
	ref, err := sdk.NewResourceRef("pixiv", []byte("artwork:1"))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	expires := time.Now().Add(time.Hour)
	res := sdk.Resource{
		Ref:                 ref,
		URL:                 "https://i.pximg.net/example",
		RequestHeaders:      map[string]string{"Referer": "https://www.pixiv.net/"},
		ExpiresAt:           &expires,
		RequiresCredentials: true,
	}
	copy := res.Copy()
	copy.RequestHeaders["Referer"] = "mutated"
	if res.RequestHeaders["Referer"] != "https://www.pixiv.net/" {
		t.Fatal("mutating copy's RequestHeaders changed original")
	}
	*copy.ExpiresAt = time.Time{}
	if res.ExpiresAt.IsZero() {
		t.Fatal("mutating copy's ExpiresAt changed original")
	}
	if copy.RequiresCredentials != res.RequiresCredentials {
		t.Fatal("scalar copy failed")
	}
}

func TestResourceResponseAllowlist(t *testing.T) {
	src := http.Header{}
	src.Set("Content-Type", "image/png")
	src.Set("Content-Length", "1024")
	src.Set("ETag", `"abc"`)
	src.Set("Location", "https://upstream/secret-path")
	src.Set("Set-Cookie", "FANBOXSESSID=leak")
	src.Set("X-Internal", "nope")
	res := sdk.NewResourceResponse(http.StatusOK, src, io.NopCloser(strings.NewReader("")))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	h := res.Header()
	if h.Get("Content-Type") != "image/png" {
		t.Fatal("allowlisted Content-Type missing")
	}
	if h.Get("Location") != "" || h.Get("Set-Cookie") != "" || h.Get("X-Internal") != "" {
		t.Fatal("non-allowlisted header leaked")
	}
	if res.ContentLength() != 1024 {
		t.Fatalf("ContentLength = %d", res.ContentLength())
	}
	if res.ETag() != `"abc"` {
		t.Fatalf("ETag = %q", res.ETag())
	}
	// Mutating the returned header must not affect the response.
	h.Set("Content-Type", "mutated")
	if res.ContentType() != "image/png" {
		t.Fatal("mutating returned Header changed response")
	}
}

func TestResourceResponseContentLengthInvalid(t *testing.T) {
	src := http.Header{}
	src.Set("Content-Length", "not-a-number")
	res := sdk.NewResourceResponse(200, src, io.NopCloser(strings.NewReader("")))
	if res.ContentLength() != 0 {
		t.Fatalf("invalid Content-Length = %d, want 0", res.ContentLength())
	}
}

func TestResourceJSONRoundTrip(t *testing.T) {
	ref, err := sdk.NewResourceRef("pixiv", []byte("ugoira:9:archive:original"))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	res := sdk.Resource{
		Ref:                 ref,
		URL:                 "https://i.pximg.net/zip/example",
		RequestHeaders:      map[string]string{"Referer": "https://www.pixiv.net/"},
		RequiresCredentials: false,
	}
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var decoded sdk.Resource
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if decoded.Ref.String() != res.Ref.String() {
		t.Fatalf("ref mismatch in JSON round-trip")
	}
	if decoded.URL != res.URL {
		t.Fatalf("url mismatch in JSON round-trip")
	}
	if decoded.RequestHeaders["Referer"] != "https://www.pixiv.net/" {
		t.Fatalf("request_headers mismatch in JSON round-trip")
	}
}
