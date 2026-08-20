package download_test

import (
	"bytes"
	"sync"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/download"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func TestDownloadProgressRendererUsesExactAggregateBytes(t *testing.T) {
	var output bytes.Buffer
	renderer := download.NewProgressRenderer(&output)
	renderer.Report(sdk.SaveProgress{Done: 50, Total: 100})
	renderer.Report(sdk.SaveProgress{Done: 100, Total: 100})
	if got, want := output.String(), "\rDownloading 50/100 bytes (50%)\rDownloading 100/100 bytes (100%)\n"; got != want {
		t.Fatalf("progress=%q want=%q", got, want)
	}
}

func TestDownloadProgressRendererSkipsUnknownAggregateSize(t *testing.T) {
	var output bytes.Buffer
	renderer := download.NewProgressRenderer(&output)
	renderer.Report(sdk.SaveProgress{Done: 50, Total: 0})
	if output.Len() != 0 {
		t.Fatalf("unknown total rendered progress: %q", output.String())
	}
}

func TestDownloadProgressRendererHandlesConcurrentCompletion(t *testing.T) {
	var output bytes.Buffer
	renderer := download.NewProgressRenderer(&output)

	const callers = 16
	var group sync.WaitGroup
	group.Add(callers)
	for range callers {
		go func() {
			defer group.Done()
			renderer.Report(sdk.SaveProgress{Done: 100, Total: 100})
		}()
	}
	group.Wait()

	if got, want := output.String(), "\rDownloading 100/100 bytes (100%)\n"; got != want {
		t.Fatalf("progress=%q want=%q", got, want)
	}
}

func TestDownloadProgressRendererIsDisabledForNonTerminalWriters(t *testing.T) {
	var output bytes.Buffer
	if renderer, ok := download.NewProgressRendererForTerminal(&output); ok || renderer != nil {
		t.Fatalf("non-terminal writer unexpectedly enabled progress renderer")
	}
}
