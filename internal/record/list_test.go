package record_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func TestRunPagesKeepsDocumentAndRecordShapes(t *testing.T) {
	pages := []sdk.Page[map[string]any]{
		{Items: []map[string]any{{"id": 1}, {"id": 2}}, Next: sdk.Cursor{}},
	}
	fetch := func(context.Context, sdk.Cursor) (sdk.Page[map[string]any], error) {
		return pages[0], nil
	}
	item := func(value map[string]any) any { return value }
	printText := func(items []map[string]any) error {
		for _, value := range items {
			fmt.Printf("id %v\n", value["id"])
		}
		return nil
	}
	plan := pagination.PagePlan{Page: 1, Limit: 2}
	for _, tc := range []struct {
		jsonOut, ndjson bool
		want            string
	}{
		{true, false, "{\n  \"posts\": [\n    {\n      \"id\": 1\n    },\n    {\n      \"id\": 2\n    }\n  ]\n}\n"},
		{false, true, "{\"id\":1}\n{\"id\":2}\n"},
	} {
		var out bytes.Buffer
		if err := record.RunPages(context.Background(), &out, plan, "posts", tc.jsonOut, tc.ndjson, printText, item, fetch); err != nil {
			t.Fatalf("RunPages: %v", err)
		}
		if out.String() != tc.want {
			t.Fatalf("output = %q, want %q", out.String(), tc.want)
		}
	}
}
