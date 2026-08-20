package diagnostics_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/diagnostics"
	core "github.com/FlanChanXwO/pixiv-cli/internal/shared/diagnostics"
)

func fixedClock() time.Time {
	return time.Date(2026, time.August, 8, 12, 21, 18, 0, time.FixedZone("CST", 8*60*60))
}

func TestPresenterRendersStableTextWithClock(t *testing.T) {
	var output bytes.Buffer
	presenter := diagnostics.NewPresenterWithClock(&output, fixedClock)
	presenter.Emit(core.Event{
		Module:    core.ModulePixivNetwork,
		Kind:      core.EventNetworkRequest,
		Operation: "retrieving",
		Resource:  "/v1/search?query=secret",
		Route:     "App API",
	})

	requireText := "[Pixiv network] 12:21:18 Request is retrieving /v1/search through the App API.\n"
	if output.String() != requireText {
		t.Fatalf("text output=%q want=%q", output.String(), requireText)
	}
}

func TestPresenterRendersSafeJSONLines(t *testing.T) {
	var output bytes.Buffer
	presenter := diagnostics.NewPresenterWithFormat(&output, "json", fixedClock)
	presenter.Emit(core.Event{
		Module:    core.ModulePixivNetwork,
		Kind:      core.EventNetworkRequest,
		Operation: "retrieving",
		Resource:  "https://user:secret@example.test/v1/search?token=secret#fragment",
		Route:     "App API",
		Proxy:     "https://proxy-user:proxy-secret@example.test:7890?token=secret",
		UserAgent: "Authorization: secret",
		Status:    200,
	})

	var record map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output.String())), &record); err != nil {
		t.Fatalf("invalid JSON diagnostics: %v", err)
	}
	if record["time"] != "2026-08-08T12:21:18+08:00" || record["level"] != "DEBUG" {
		t.Fatalf("unexpected metadata: %#v", record)
	}
	if record["resource"] != "https://example.test/v1/search" || record["proxy"] != "https://example.test:7890" {
		t.Fatalf("unsafe URL fields were not sanitized: %#v", record)
	}
	for _, secret := range []string{"secret", "proxy-user", "Authorization", "user"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("diagnostic output leaked %q: %s", secret, output.String())
		}
	}
	if _, ok := record["user_agent"]; ok {
		t.Fatal("diagnostic JSON must not include headers")
	}
}

func TestPresenterRetainsFirstWriterError(t *testing.T) {
	writer := &errorWriter{err: errors.New("diagnostic sink closed")}
	presenter := diagnostics.NewPresenter(writer)
	presenter.Emit(core.Event{Module: core.ModulePixivCLI, Kind: core.EventStarted, Operation: "pixiv version"})
	presenter.Emit(core.Event{Module: core.ModulePixivCLI, Kind: core.EventCompleted, Operation: "pixiv version"})
	if !errors.Is(presenter.Err(), writer.err) {
		t.Fatalf("presenter error=%v want %v", presenter.Err(), writer.err)
	}
}

type errorWriter struct{ err error }

func (w *errorWriter) Write([]byte) (int, error) { return 0, w.err }
