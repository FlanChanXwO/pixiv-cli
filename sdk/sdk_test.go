package sdk_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/internal/cursor"
)

func TestCursorQueryBinding(t *testing.T) {
	original := cursor.Binding{Product: "pixiv", Operation: "search", Account: "11", Query: url.Values{"word": {"private-search"}}}
	token, err := cursor.Encode(original, url.Values{"offset": {"30"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"query", "operation", "account", "product"} {
		t.Run(field, func(t *testing.T) {
			changed := original
			switch field {
			case "query":
				changed.Query = url.Values{"word": {"different"}}
			case "operation":
				changed.Operation = "bookmarks"
			case "account":
				changed.Account = "12"
			case "product":
				changed.Product = "fanbox"
			}
			if _, err := cursor.Decode(token, changed); sdk.ReasonOf(err) != sdk.InvalidArgument {
				t.Fatalf("mismatched %s: %v", field, err)
			}
		})
	}
}

func TestUnboundPage(t *testing.T) {
	token, err := cursor.Encode(cursor.Binding{Product: "pixiv", Operation: "search"}, url.Values{"offset": {"30"}})
	if err != nil {
		t.Fatal(err)
	}
	page := sdk.NewPage([]int{1}, token, func(context.Context) (sdk.Page[int], error) {
		t.Fatal("decoded page retained a previous query binding")
		return sdk.Page[int]{}, nil
	})
	data, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &page); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Next(t.Context()); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("unbound: %v", err)
	}
}

func TestErrorChainAndPrivacy(t *testing.T) {
	failure := &sdk.Error{Reason: sdk.LocalStateError, Operation: "save", Message: "credential save failed", Err: context.Canceled}
	if !errors.Is(failure, context.Canceled) || !errors.Is(failure, &sdk.Error{Reason: sdk.LocalStateError}) {
		t.Fatal("lost classification or cause")
	}
	if sdk.ReasonOf(fmt.Errorf("wrapped: %w", failure)) != sdk.LocalStateError {
		t.Fatal("lost wrapped reason")
	}
	failure.Err = errors.New("secret-fixture")
	for _, format := range []string{"%v", "%+v", "%#v", "%q"} {
		if strings.Contains(fmt.Sprintf(format, failure), "secret-fixture") {
			t.Fatal("cause leaked in summary")
		}
	}
	if errors.Unwrap(failure).Error() != "secret-fixture" {
		t.Fatal("cause was discarded")
	}
}
