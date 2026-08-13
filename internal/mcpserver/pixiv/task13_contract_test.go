package pixiv_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestTask13DownloadSchemaPublishesOnlyMappedOptions(t *testing.T) {
	session, closeSession := newTestSession(t, &fakeDownloads{})
	defer closeSession()

	var rawSchema any
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		if tool.Name == "download" {
			rawSchema = tool.InputSchema
			break
		}
	}
	if rawSchema == nil {
		t.Fatal("download tool not found")
	}
	raw, err := json.Marshal(rawSchema)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"src", "srcs", "pages", "quality", "ugoira_mode", "delivery"} {
		if !strings.Contains(string(raw), `"`+field+`"`) {
			t.Fatalf("download schema missing mapped field %q: %s", field, raw)
		}
	}
	for _, field := range []string{"concurrency", "filter", "archive", "directory_template", "write_metadata", "retries", "retry_delay"} {
		if strings.Contains(string(raw), `"`+field+`"`) {
			t.Fatalf("download schema publishes unmapped field %q: %s", field, raw)
		}
	}
}
