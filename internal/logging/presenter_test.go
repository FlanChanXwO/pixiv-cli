package logging_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
)

func fixedClock() time.Time {
	return time.Date(2026, time.August, 8, 12, 21, 18, 0, time.FixedZone("CST", 8*60*60))
}

func TestPresenterRendersStableTextWithClock(t *testing.T) {
	var output bytes.Buffer
	presenter := logging.NewPresenterWithClock(&output, fixedClock)
	presenter.Emit(logging.Event{
		Module:    logging.ModulePixivNetwork,
		Kind:      logging.EventNetworkRequest,
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
	presenter := logging.NewPresenterWithFormat(&output, "json", fixedClock)
	presenter.Emit(logging.Event{
		Module:    logging.ModulePixivNetwork,
		Kind:      logging.EventNetworkRequest,
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
	presenter := logging.NewPresenter(writer)
	presenter.Emit(logging.Event{Module: logging.ModulePixivCLI, Kind: logging.EventStarted, Operation: "pixiv version"})
	presenter.Emit(logging.Event{Module: logging.ModulePixivCLI, Kind: logging.EventCompleted, Operation: "pixiv version"})
	if !errors.Is(presenter.Err(), writer.err) {
		t.Fatalf("presenter error=%v want %v", presenter.Err(), writer.err)
	}
}

type errorWriter struct{ err error }

func (w *errorWriter) Write([]byte) (int, error) { return 0, w.err }

func TestPresenterExplainsMediaInterruption(t *testing.T) {
	for _, tc := range []struct {
		reason logging.Reason
		text   string
	}{
		{logging.Reason("request cancelled"), "the request was cancelled"},
		{logging.Reason("transport interrupted"), "the transport was interrupted"},
		{logging.Reason("subscription released"), "the last subscription was released"},
		{logging.Reason("service closed"), "the service was closed"},
		{logging.Reason("authorization expired"), "the media authorization expired"},
		{logging.Reason("authorization invalidated"), "the media authorization was invalidated"},
	} {
		var output bytes.Buffer
		presenter := logging.NewPresenterWithClock(&output, fixedClock)
		presenter.Emit(logging.Event{Module: logging.ModulePixivMCP, Kind: logging.EventFailed, Operation: "animation_delivery", Reason: tc.reason})
		if !strings.Contains(output.String(), tc.text) {
			t.Fatalf("missing cancellation reason: %q", output.String())
		}
	}
}
