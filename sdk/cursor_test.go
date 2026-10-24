package sdk_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func TestCursorRoundTrip(t *testing.T) {
	payload := []byte("continuation-state-without-secrets")
	c, err := sdk.NewCursor("pixiv", "UserArtworkBookmarks", 1, "digest123", payload)
	if err != nil {
		t.Fatalf("NewCursor: %v", err)
	}
	text, err := c.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	parsed, err := sdk.ParseCursor(string(text))
	if err != nil {
		t.Fatalf("ParseCursor: %v", err)
	}
	if err := sdk.ValidateCursor(parsed, "pixiv", "UserArtworkBookmarks", 1, "digest123"); err != nil {
		t.Fatalf("ValidateCursor: %v", err)
	}
	got, err := sdk.CursorPayload(parsed)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("payload = %q err=%v", got, err)
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var decoded sdk.Cursor
	if err := json.Unmarshal(data, &decoded); err != nil || decoded.String() != c.String() {
		t.Fatalf("JSON round-trip = %q err=%v", decoded.String(), err)
	}
}

func TestCursorTextIsRouteSafeAndDeterministic(t *testing.T) {
	c, err := sdk.NewCursor("pixiv", "SearchArtworks", 1, "digest", []byte{0, 1, 2, 255})
	if err != nil {
		t.Fatalf("NewCursor: %v", err)
	}
	text := c.String()
	for _, r := range text {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			t.Fatalf("cursor text contains non-route-safe char %q", r)
		}
	}
	if strings.Contains(text, string([]byte{0, 1, 2, 255})) {
		t.Fatal("cursor text leaked raw payload bytes")
	}
	again, err := sdk.NewCursor("pixiv", "SearchArtworks", 1, "digest", []byte{0, 1, 2, 255})
	if err != nil || again.String() != text {
		t.Fatalf("cursor encoding not deterministic: %q vs %q", again.String(), text)
	}
}

func TestCursorRejectsInvalidInput(t *testing.T) {
	invalidNew := []struct {
		product, operation, digest string
		binding                    int
		payload                    []byte
	}{
		{product: "", operation: "Op", binding: 1, digest: "q", payload: []byte("p")},
		{product: "pixiv", operation: "", binding: 1, digest: "q", payload: []byte("p")},
		{product: "pixiv", operation: "Op", binding: 0, digest: "q", payload: []byte("p")},
		{product: "pixiv", operation: "Op", binding: 1, digest: "", payload: []byte("p")},
		{product: "pixiv", operation: "Op", binding: 1, digest: "q", payload: nil},
	}
	for _, test := range invalidNew {
		if _, err := sdk.NewCursor(test.product, test.operation, test.binding, test.digest, test.payload); sdk.ReasonOf(err) != sdk.InvalidArgument {
			t.Fatalf("NewCursor(%#v) = %v, want InvalidArgument", test, err)
		}
	}
	for _, in := range []string{"", "%%%", "not-base64url!!!", "aGVsbG8"} {
		if _, err := sdk.ParseCursor(in); sdk.ReasonOf(err) != sdk.InvalidCursor {
			t.Fatalf("ParseCursor(%q) = %v, want InvalidCursor", in, err)
		}
	}

	c, err := sdk.NewCursor("pixiv", "SearchArtworks", 1, "digest", []byte("p"))
	if err != nil {
		t.Fatalf("NewCursor: %v", err)
	}
	for _, test := range []struct {
		product, operation, digest string
		binding                    int
	}{
		{product: "fanbox", operation: "SearchArtworks", binding: 1, digest: "digest"},
		{product: "pixiv", operation: "Artwork", binding: 1, digest: "digest"},
		{product: "pixiv", operation: "SearchArtworks", binding: 2, digest: "digest"},
		{product: "pixiv", operation: "SearchArtworks", binding: 1, digest: "other"},
	} {
		if err := sdk.ValidateCursor(c, test.product, test.operation, test.binding, test.digest); sdk.ReasonOf(err) != sdk.InvalidCursor {
			t.Fatalf("ValidateCursor(%#v) = %v, want InvalidCursor", test, err)
		}
	}
	if err := sdk.ValidateCursor(sdk.Cursor{}, "pixiv", "SearchArtworks", 1, "digest"); sdk.ReasonOf(err) != sdk.InvalidCursor {
		t.Fatalf("zero cursor validation = %v", err)
	}
	var zero sdk.Cursor
	if !zero.IsZero() {
		t.Fatal("zero cursor IsZero should be true")
	}
	if _, err := zero.MarshalText(); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("zero cursor marshal = %v", err)
	}
	if _, err := json.Marshal(zero); err == nil {
		t.Fatal("zero cursor JSON marshal should error")
	}
}

func TestCursorIdentityAndEphemeralBinding(t *testing.T) {
	c, err := sdk.NewCursor("pixiv", "UserArtworkBookmarks", 1, "digest", []byte("p"),
		sdk.WithCursorIdentity("user:42"),
		sdk.WithCursorEphemeral(),
	)
	if err != nil {
		t.Fatalf("NewCursor: %v", err)
	}
	identity, ok := sdk.CursorIdentity(c)
	if !ok || identity != "user:42" || !sdk.CursorEphemeral(c) {
		t.Fatalf("identity = %q, %v ephemeral=%v", identity, ok, sdk.CursorEphemeral(c))
	}
	plain, err := sdk.NewCursor("pixiv", "SearchArtworks", 1, "digest", []byte("p"))
	if err != nil {
		t.Fatalf("NewCursor: %v", err)
	}
	if _, ok := sdk.CursorIdentity(plain); ok || sdk.CursorEphemeral(plain) {
		t.Fatal("unexpected identity or ephemeral flag on plain cursor")
	}

	instance, err := sdk.NewCursor("pixiv", "FollowingArtworks", 1, "digest", []byte("p"), sdk.WithCursorEphemeralInstance("instance-a"))
	if err != nil {
		t.Fatalf("NewCursor: %v", err)
	}
	if !sdk.CursorEphemeral(instance) || sdk.ValidateCursorInstance(instance, "instance-a") != nil {
		t.Fatal("same instance should validate")
	}
	if err := sdk.ValidateCursorInstance(instance, "instance-b"); sdk.ReasonOf(err) != sdk.InvalidCursor {
		t.Fatalf("different instance = %v, want InvalidCursor", err)
	}
	if _, err := sdk.NewCursor("pixiv", "FollowingArtworks", 1, "digest", []byte("p"), sdk.WithCursorEphemeralInstance("")); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("empty instance = %v, want InvalidArgument", err)
	}
}
