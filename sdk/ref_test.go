package sdk_test

import (
	"encoding/json"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func TestResourceRefRoundTripIsOpaqueAndRouteSafe(t *testing.T) {
	ref, err := sdk.NewResourceRef("pixiv", []byte("artwork:42:page:0"))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	text, err := ref.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	parsed, err := sdk.ParseResourceRef(string(text))
	if err != nil {
		t.Fatalf("ParseResourceRef: %v", err)
	}
	if parsed != ref {
		t.Fatalf("round-trip mismatch: %q vs %q", parsed.String(), ref.String())
	}
	product, err := sdk.ResourceRefProduct(parsed)
	if err != nil || product != "pixiv" {
		t.Fatalf("product = %q err=%v", product, err)
	}
	payload, err := sdk.ResourceRefPayload(parsed)
	if err != nil || string(payload) != "artwork:42:page:0" {
		t.Fatalf("payload = %q err=%v", payload, err)
	}
	for _, r := range ref.String() {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			t.Fatalf("ref text contains non-route-safe char %q", r)
		}
	}

	data, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var decoded sdk.ResourceRef
	if err := json.Unmarshal(data, &decoded); err != nil || decoded.String() != ref.String() {
		t.Fatalf("JSON round-trip mismatch: %v", err)
	}
}

func TestResourceRefRejectsInvalidAndZero(t *testing.T) {
	if _, err := sdk.NewResourceRef("", []byte("p")); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("empty product = %v", err)
	}
	if _, err := sdk.NewResourceRef("pixiv", nil); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("empty payload = %v", err)
	}
	for _, in := range []string{"", "!!!", "YWJjZA", "%%%%"} {
		if _, err := sdk.ParseResourceRef(in); sdk.ReasonOf(err) != sdk.InvalidArgument {
			t.Fatalf("ParseResourceRef(%q) = %v", in, err)
		}
	}
	var zero sdk.ResourceRef
	if !zero.IsZero() {
		t.Fatal("zero ref IsZero should be true")
	}
	if _, err := zero.MarshalText(); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("zero ref marshal = %v", err)
	}
	if _, err := json.Marshal(zero); err == nil {
		t.Fatal("zero ref JSON marshal should error")
	}
}
