package diagnostics_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/diagnostics"
	core "github.com/FlanChanXwO/pixiv-cli/internal/diagnostics"
)

func TestPresenterUsesStableNarrativeAndClock(t *testing.T) {
	var output bytes.Buffer
	clock := func() time.Time {
		return time.Date(2026, time.August, 8, 12, 21, 18, 0, time.FixedZone("CST", 8*60*60))
	}
	presenter := diagnostics.NewPresenterWithClock(&output, clock)

	presenter.Emit(core.Event{Module: core.ModuleFanboxMCPServer, Kind: core.EventStarted, Operation: "tool fanbox_get_post", RequestID: 7})
	presenter.Emit(core.Event{Module: core.ModuleFanboxNetwork, Kind: core.EventNetworkRequest, Operation: "retrieving", Resource: "post 12221352", Route: "native transport", RequestID: 7})
	presenter.Emit(core.Event{Module: core.ModuleFanboxNetwork, Kind: core.EventChallenge, Operation: "request", Status: 403, RequestID: 7})
	presenter.Emit(core.Event{Module: core.ModuleFanboxSolver, Kind: core.EventSolverStarted, Operation: "challenge recovery", RequestID: 7})
	presenter.Emit(core.Event{Module: core.ModuleFanboxSolver, Kind: core.EventSolverCompleted, Operation: "clearance", RequestID: 7})
	presenter.Emit(core.Event{Module: core.ModuleFanboxNetwork, Kind: core.EventReplay, Operation: "request", Route: "native transport", RequestID: 7})
	presenter.Emit(core.Event{Module: core.ModuleFanboxMCPServer, Kind: core.EventCompleted, Operation: "tool fanbox_get_post", RequestID: 7, Duration: 16*time.Second + 200*time.Millisecond})

	want := strings.Join([]string{
		"[FANBOX MCP server] 12:21:18 Started request 7 for tool fanbox_get_post.",
		"[FANBOX network] 12:21:18 Request 7 is retrieving post 12221352 through the native transport.",
		"[FANBOX network] 12:21:18 Cloudflare challenged request 7 with HTTP 403.",
		"[FANBOX FlareSolverr] 12:21:18 Request 7 requires fresh Cloudflare clearance.",
		"[FANBOX FlareSolverr] 12:21:18 Clearance was acquired; request 7 will be replayed natively.",
		"[FANBOX network] 12:21:18 Request 7 is replaying through the native transport.",
		"[FANBOX MCP server] 12:21:18 Request 7 completed successfully in 16.2 seconds.",
	}, "\n") + "\n"
	if output.String() != want {
		t.Fatalf("narrative output=\n%s\nwant=\n%s", output.String(), want)
	}
}

func TestPresenterRemovesSensitiveURLParts(t *testing.T) {
	var output bytes.Buffer
	presenter := diagnostics.NewPresenterWithClock(&output, func() time.Time { return time.Unix(0, 0) })
	presenter.Emit(core.Event{
		Module:    core.ModuleFanboxNetwork,
		Kind:      core.EventNetworkRequest,
		Operation: "retrieving",
		Resource:  "https://user:secret@example.test/post/1?signature=secret#fragment",
		Route:     "native transport",
		Proxy:     "https://proxy-user:proxy-secret@proxy.test:7890?token=secret",
		UserAgent: "Mozilla/5.0\nAuthorization: secret",
	})

	got := output.String()
	for _, secret := range []string{"secret", "proxy-user", "Authorization:"} {
		if strings.Contains(got, secret) {
			t.Fatalf("diagnostic output leaked %q: %s", secret, got)
		}
	}
	if strings.Contains(got, "?signature=") || strings.Contains(got, "#fragment") {
		t.Fatalf("diagnostic output retained signed URL parts: %s", got)
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
