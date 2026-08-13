package sdk_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func TestCursorRoundTripPreservesBindingAndPayload(t *testing.T) {
	payload := []byte("continuation-state-without-secrets")
	c, err := sdk.NewCursor("pixiv", "UserArtworkBookmarks", 1, "digest123", payload)
	if err != nil {
		t.Fatalf("NewCursor: %v", err)
	}
	if c.IsZero() {
		t.Fatal("cursor unexpectedly zero")
	}

	text, err := c.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	parsed, err := sdk.ParseCursor(string(text))
	if err != nil {
		t.Fatalf("ParseCursor: %v", err)
	}
	if parsed.IsZero() {
		t.Fatal("parsed cursor unexpectedly zero")
	}
	if err := sdk.ValidateCursor(parsed, "pixiv", "UserArtworkBookmarks", 1, "digest123"); err != nil {
		t.Fatalf("ValidateCursor on round-tripped cursor: %v", err)
	}
	got, err := sdk.CursorPayload(parsed)
	if err != nil {
		t.Fatalf("CursorPayload: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload mismatch: got %q want %q", got, payload)
	}
}

func TestCursorTextIsRouteSafe(t *testing.T) {
	c, err := sdk.NewCursor("pixiv", "SearchArtworks", 1, "digest", []byte{0, 1, 2, 255})
	if err != nil {
		t.Fatalf("NewCursor: %v", err)
	}
	text := c.String()
	if text == "" {
		t.Fatal("empty cursor text")
	}
	for _, r := range text {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			t.Fatalf("cursor text contains non-route-safe char %q", r)
		}
	}
	if strings.Contains(text, string([]byte{0, 1, 2, 255})) {
		t.Fatal("cursor text leaked raw payload bytes")
	}
}

func TestCursorTextIsDeterministic(t *testing.T) {
	c, err := sdk.NewCursor("pixiv", "SearchNovels", 2, "digest", []byte("p"))
	if err != nil {
		t.Fatalf("NewCursor: %v", err)
	}
	again, err := sdk.NewCursor("pixiv", "SearchNovels", 2, "digest", []byte("p"))
	if err != nil {
		t.Fatalf("NewCursor: %v", err)
	}
	if c.String() != again.String() {
		t.Fatalf("cursor encoding not deterministic: %q vs %q", c.String(), again.String())
	}
}

func TestNewCursorRejectsInvalidBindings(t *testing.T) {
	cases := []struct {
		name      string
		product   string
		op        string
		binding   int
		queryHash string
		payload   []byte
	}{
		{name: "empty product", product: "", op: "Op", binding: 1, queryHash: "q", payload: []byte("p")},
		{name: "empty operation", product: "pixiv", op: "", binding: 1, queryHash: "q", payload: []byte("p")},
		{name: "zero binding", product: "pixiv", op: "Op", binding: 0, queryHash: "q", payload: []byte("p")},
		{name: "negative binding", product: "pixiv", op: "Op", binding: -1, queryHash: "q", payload: []byte("p")},
		{name: "empty query hash", product: "pixiv", op: "Op", binding: 1, queryHash: "", payload: []byte("p")},
		{name: "empty payload", product: "pixiv", op: "Op", binding: 1, queryHash: "q", payload: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := sdk.NewCursor(tc.product, tc.op, tc.binding, tc.queryHash, tc.payload)
			if sdk.ReasonOf(err) != sdk.InvalidArgument {
				t.Fatalf("expected InvalidArgument, got %v", err)
			}
		})
	}
}

func TestCursorValidationRejectsMismatch(t *testing.T) {
	c, err := sdk.NewCursor("pixiv", "SearchArtworks", 1, "digest", []byte("p"))
	if err != nil {
		t.Fatalf("NewCursor: %v", err)
	}
	cases := []struct {
		name      string
		product   string
		op        string
		binding   int
		queryHash string
	}{
		{name: "wrong product", product: "fanbox", op: "SearchArtworks", binding: 1, queryHash: "digest"},
		{name: "wrong operation", product: "pixiv", op: "Artwork", binding: 1, queryHash: "digest"},
		{name: "wrong binding", product: "pixiv", op: "SearchArtworks", binding: 2, queryHash: "digest"},
		{name: "wrong query", product: "pixiv", op: "SearchArtworks", binding: 1, queryHash: "other"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := sdk.ValidateCursor(c, tc.product, tc.op, tc.binding, tc.queryHash); sdk.ReasonOf(err) != sdk.InvalidCursor {
				t.Fatalf("expected InvalidCursor, got %v", err)
			}
		})
	}
	if err := sdk.ValidateCursor(sdk.Cursor{}, "pixiv", "SearchArtworks", 1, "digest"); sdk.ReasonOf(err) != sdk.InvalidCursor {
		t.Fatalf("zero cursor validation: expected InvalidCursor, got %v", err)
	}
}

func TestParseCursorRejectsGarbage(t *testing.T) {
	cases := []string{
		"",
		"%%%",
		"not-base64url!!!",
		"aGVsbG8",
	}
	for _, in := range cases {
		if _, err := sdk.ParseCursor(in); sdk.ReasonOf(err) != sdk.InvalidCursor {
			t.Fatalf("ParseCursor(%q): expected InvalidCursor, got %v", in, err)
		}
	}
}

func TestCursorJSONRoundTrip(t *testing.T) {
	c, err := sdk.NewCursor("pixiv", "ArtworkRanking", 1, "digest", []byte("payload"))
	if err != nil {
		t.Fatalf("NewCursor: %v", err)
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var decoded sdk.Cursor
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if decoded.String() != c.String() {
		t.Fatalf("JSON round-trip mismatch: %q vs %q", decoded.String(), c.String())
	}
}

func TestCursorZeroValue(t *testing.T) {
	var c sdk.Cursor
	if !c.IsZero() {
		t.Fatal("zero cursor IsZero should be true")
	}
	if _, err := c.MarshalText(); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("zero cursor marshal: expected InvalidArgument, got %v", err)
	}
	if _, err := json.Marshal(c); err == nil {
		t.Fatal("zero cursor JSON marshal should error")
	}
}

func TestCursorIdentityAndEphemeralRoundTrip(t *testing.T) {
	c, err := sdk.NewCursor("pixiv", "UserArtworkBookmarks", 1, "digest", []byte("p"),
		sdk.WithCursorIdentity("user:42"),
		sdk.WithCursorEphemeral(),
	)
	if err != nil {
		t.Fatalf("NewCursor: %v", err)
	}
	identity, ok := sdk.CursorIdentity(c)
	if !ok || identity != "user:42" {
		t.Fatalf("identity = %q, %v; want user:42, true", identity, ok)
	}
	if !sdk.CursorEphemeral(c) {
		t.Fatal("cursor should be ephemeral")
	}

	text, err := c.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	parsed, err := sdk.ParseCursor(string(text))
	if err != nil {
		t.Fatalf("ParseCursor: %v", err)
	}
	identity, ok = sdk.CursorIdentity(parsed)
	if !ok || identity != "user:42" {
		t.Fatalf("round-tripped identity = %q, %v; want user:42, true", identity, ok)
	}
	if !sdk.CursorEphemeral(parsed) {
		t.Fatal("round-tripped cursor should be ephemeral")
	}
}

func TestCursorIdentityAbsentByDefault(t *testing.T) {
	c, err := sdk.NewCursor("pixiv", "SearchArtworks", 1, "digest", []byte("p"))
	if err != nil {
		t.Fatalf("NewCursor: %v", err)
	}
	if _, ok := sdk.CursorIdentity(c); ok {
		t.Fatal("unexpected identity on search cursor")
	}
	if sdk.CursorEphemeral(c) {
		t.Fatal("unexpected ephemeral flag")
	}
}

func TestCursorEphemeralInstanceBinding(t *testing.T) {
	c, err := sdk.NewCursor("pixiv", "FollowingArtworks", 1, "digest", []byte("p"),
		sdk.WithCursorEphemeralInstance("instance-a"),
	)
	if err != nil {
		t.Fatalf("NewCursor: %v", err)
	}
	if !sdk.CursorEphemeral(c) {
		t.Fatal("cursor should be ephemeral")
	}
	if err := sdk.ValidateCursorInstance(c, "instance-a"); err != nil {
		t.Fatalf("same instance should validate: %v", err)
	}
	if err := sdk.ValidateCursorInstance(c, "instance-b"); sdk.ReasonOf(err) != sdk.InvalidCursor {
		t.Fatalf("different instance should return InvalidCursor, got %v", err)
	}
	text, err := c.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	parsed, err := sdk.ParseCursor(string(text))
	if err != nil {
		t.Fatalf("ParseCursor: %v", err)
	}
	if err := sdk.ValidateCursorInstance(parsed, "instance-a"); err != nil {
		t.Fatalf("round-tripped instance should validate: %v", err)
	}
}

func TestCursorEphemeralInstanceRequiresIdentifier(t *testing.T) {
	_, err := sdk.NewCursor("pixiv", "FollowingArtworks", 1, "digest", []byte("p"),
		sdk.WithCursorEphemeralInstance(""),
	)
	if sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("empty instance should return InvalidArgument, got %v", err)
	}
}
