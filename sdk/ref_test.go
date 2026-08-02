package sdk_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func TestResourceRefRoundTrip(t *testing.T) {
	ref, err := sdk.NewResourceRef("pixiv", []byte("artwork:42:page:0"))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	if ref.IsZero() {
		t.Fatal("ref unexpectedly zero")
	}
	text, err := ref.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	parsed, err := sdk.ParseResourceRef(string(text))
	if err != nil {
		t.Fatalf("ParseResourceRef: %v", err)
	}
	if parsed.String() != ref.String() {
		t.Fatalf("round-trip mismatch: %q vs %q", parsed.String(), ref.String())
	}
	product, err := sdk.ResourceRefProduct(parsed)
	if err != nil {
		t.Fatalf("ResourceRefProduct: %v", err)
	}
	if product != "pixiv" {
		t.Fatalf("product = %q", product)
	}
	payload, err := sdk.ResourceRefPayload(parsed)
	if err != nil {
		t.Fatalf("ResourceRefPayload: %v", err)
	}
	if string(payload) != "artwork:42:page:0" {
		t.Fatalf("payload = %q", payload)
	}
}

func TestResourceRefTextIsRouteSafe(t *testing.T) {
	ref, err := sdk.NewResourceRef("pixiv", []byte{0x00, 0xff, 0x01})
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	for _, r := range ref.String() {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			t.Fatalf("ref text contains non-route-safe char %q", r)
		}
	}
	if strings.Contains(ref.String(), string([]byte{0x00, 0xff, 0x01})) {
		t.Fatal("ref text leaked raw payload")
	}
}

func TestResourceRefJSONRoundTrip(t *testing.T) {
	ref, err := sdk.NewResourceRef("fanbox", []byte("post-asset:7"))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	data, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var decoded sdk.ResourceRef
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if decoded.String() != ref.String() {
		t.Fatalf("JSON round-trip mismatch")
	}
}

func TestResourceRefRejectsInvalid(t *testing.T) {
	if _, err := sdk.NewResourceRef("", []byte("p")); sdk.CodeOf(err) != sdk.CodeInvalidArgument {
		t.Fatalf("empty product: expected CodeInvalidArgument, got %v", err)
	}
	if _, err := sdk.NewResourceRef("pixiv", nil); sdk.CodeOf(err) != sdk.CodeInvalidArgument {
		t.Fatalf("empty payload: expected CodeInvalidArgument, got %v", err)
	}
	for _, in := range []string{"", "!!!", "YWJjZA", "%%%%"} {
		if _, err := sdk.ParseResourceRef(in); sdk.CodeOf(err) != sdk.CodeInvalidArgument {
			t.Fatalf("ParseResourceRef(%q): expected CodeInvalidArgument, got %v", in, err)
		}
	}
}

func TestResourceRefZero(t *testing.T) {
	var ref sdk.ResourceRef
	if !ref.IsZero() {
		t.Fatal("zero ref IsZero should be true")
	}
	if _, err := ref.MarshalText(); sdk.CodeOf(err) != sdk.CodeInvalidArgument {
		t.Fatalf("zero ref marshal: expected CodeInvalidArgument, got %v", err)
	}
	if _, err := json.Marshal(ref); err == nil {
		t.Fatal("zero ref JSON marshal should error")
	}
}

func TestResourceRefIsComparable(t *testing.T) {
	ref, err := sdk.NewResourceRef("pixiv", []byte("stable-identity"))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	a, err := sdk.ParseResourceRef(ref.String())
	if err != nil {
		t.Fatalf("ParseResourceRef: %v", err)
	}
	b, err := sdk.ParseResourceRef(ref.String())
	if err != nil {
		t.Fatalf("ParseResourceRef: %v", err)
	}
	if a != b {
		t.Fatal("equal refs should be comparable and equal")
	}
}
