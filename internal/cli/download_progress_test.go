package cli

import (
	"bytes"
	"sync"
	"testing"

	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestDownloadProgressRendererUsesExactAggregateBytes(t *testing.T) {
	var output bytes.Buffer
	renderer := &downloadProgressRenderer{writer: &output}
	renderer.Report(sdk.DownloadProgress{TotalBytesKnown: true, TotalBytesTransferred: 50, TotalBytes: 100, CompletedResources: 0, TotalResources: 1})
	renderer.Report(sdk.DownloadProgress{TotalBytesKnown: true, TotalBytesTransferred: 100, TotalBytes: 100, CompletedResources: 1, TotalResources: 1})
	if got, want := output.String(), "\rDownloading 50/100 bytes (50%)\rDownloading 100/100 bytes (100%)\n"; got != want {
		t.Fatalf("progress=%q want=%q", got, want)
	}
}

func TestDownloadProgressRendererSkipsUnknownAggregateSize(t *testing.T) {
	var output bytes.Buffer
	renderer := &downloadProgressRenderer{writer: &output}
	renderer.Report(sdk.DownloadProgress{TotalBytesTransferred: 50, TotalBytesKnown: false, TotalResources: 1})
	if output.Len() != 0 {
		t.Fatalf("unknown total rendered progress: %q", output.String())
	}
}

func TestDownloadProgressRendererHandlesConcurrentCompletion(t *testing.T) {
	var output bytes.Buffer
	renderer := &downloadProgressRenderer{writer: &output}

	const callers = 16
	var group sync.WaitGroup
	group.Add(callers)
	for range callers {
		go func() {
			defer group.Done()
			renderer.Report(sdk.DownloadProgress{
				TotalBytesKnown: true, TotalBytesTransferred: 100, TotalBytes: 100,
				CompletedResources: 1, TotalResources: 1,
			})
		}()
	}
	group.Wait()

	if got, want := output.String(), "\rDownloading 100/100 bytes (100%)\n"; got != want {
		t.Fatalf("progress=%q want=%q", got, want)
	}
}

func TestDownloadProgressRendererIsDisabledForNonTerminalWriters(t *testing.T) {
	var output bytes.Buffer
	if renderer, ok := newDownloadProgressRenderer(&output); ok || renderer != nil {
		t.Fatalf("non-terminal writer unexpectedly enabled progress renderer: %#v", renderer)
	}
}
