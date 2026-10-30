package record_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
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
		if err := record.RunPages(context.Background(), &out, io.Discard, plan, "posts", tc.jsonOut, tc.ndjson, printText, item, fetch); err != nil {
			t.Fatalf("RunPages: %v", err)
		}
		if out.String() != tc.want {
			t.Fatalf("output = %q, want %q", out.String(), tc.want)
		}
	}
}

func TestRunArtworksClosesSpoolOnReplay(t *testing.T) {
	// 把临时目录隔离到本测试内，避免与其他并行测试创建的同名 spool 冲突。
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)
	var out bytes.Buffer
	runner := session.NewRunner(&out, func(ctx context.Context, _ session.ProxyRequest, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		_, err := attempt(ctx, nil)
		return err
	})
	attempts := 0
	before := spoolNames(t, tempDir)
	err := record.RunArtworks(context.Background(), runner, session.ProxyRequest{}, pagination.PagePlan{Page: 1, Limit: 1}, record.FormatJSON, io.Discard, record.ArtworkList{
		Fetch: func(*pixiv.Client, context.Context, sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			attempts++
			if attempts == 1 {
				return nil, sdk.Cursor{}, retryableErr
			}
			return []pixiv.Artwork{{ID: 7, Title: "cat"}}, sdk.Cursor{}, nil
		},
	})
	if !errors.Is(err, retryableErr) || out.Len() != 0 {
		t.Fatalf("err = %v, stdout = %q; want the retryable failure and empty stdout", err, out.String())
	}
	if after := spoolNames(t, tempDir); len(after) != len(before) {
		t.Fatalf("spool files leaked: before=%v after=%v", before, after)
	}
}

var retryableErr = errors.New("retry")

func spoolNames(t *testing.T, dir string) []string {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(dir, "pixiv-cli-json-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	return names
}
