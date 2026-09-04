package download_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	download "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/download"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestCommandRejectsProxyConflictBeforeResolvingDownloadServices(t *testing.T) {
	resolved := false
	cmd := download.New(download.Deps{
		Input:       strings.NewReader(""),
		Output:      &bytes.Buffer{},
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		Runtime: func() (download.Runtime, error) {
			return download.Runtime{}, errors.New("runtime must not be loaded")
		},
		Download: func() downloader.DownloadService {
			resolved = true
			return downloader.DownloadService{}
		},
		Pooled: func(context.Context, download.CommandRequest, func(context.Context, *pixiv.Client) (bool, error)) error {
			return errors.New("pooled client must not be opened")
		},
	})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"42", "--proxy", "http://127.0.0.1:1", "--no-proxy"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "either --proxy or --no-proxy") {
		t.Fatalf("expected proxy conflict, got %v", err)
	}
	if resolved {
		t.Fatal("download service was resolved before validating command flags")
	}
}

func TestCommandConsumesBatchReportWithWarningsAndFailures(t *testing.T) {
	failureCause := errors.New("page 3 failed")
	manager := &cliDownloadManagerStub{result: downloader.DownloadBatchResult{
		Items: []downloader.DownloadedArtwork{{
			IllustID: 42,
			Title:    "multi-page",
			Author:   "artist",
			Type:     "illust",
			Files: []downloader.DownloadedFile{
				{Path: "/tmp/42-1.jpg", Page: 1},
				{Path: "/tmp/42-2.jpg", Page: 2},
			},
		}},
		Failures: []downloader.DownloadFailure{{
			IllustID: 42,
			Type:     "illust",
			Message:  failureCause.Error(),
			Cause:    failureCause,
		}},
		Warnings: []downloader.DownloadWarning{{
			IllustID: 42,
			Type:     "ugoira",
			Message:  "using default filename",
		}},
	}}
	client, err := pixiv.New("test-access-token")
	if err != nil {
		t.Fatalf("pixiv.New: %v", err)
	}
	t.Cleanup(client.CloseIdleConnections)

	var output, errorOutput bytes.Buffer
	var committed bool
	cmd := download.New(download.Deps{
		Input:       strings.NewReader(""),
		Output:      &output,
		ErrorOutput: &errorOutput,
		UsageError:  func(err error) error { return err },
		Runtime: func() (download.Runtime, error) {
			return download.Runtime{DownloadPath: t.TempDir()}, nil
		},
		Download: func() downloader.DownloadService {
			return downloader.DownloadService{
				NewManager: func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
					return manager, nil
				},
			}
		},
		Pooled: func(ctx context.Context, _ download.CommandRequest, invoke func(context.Context, *pixiv.Client) (bool, error)) error {
			committed, err = invoke(ctx, client)
			return err
		},
	})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"42", "--pages", "1,3-4", "--quality", "small", "--ugoira-mode", "apng"})

	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "download completed with 1 failures") {
		t.Fatalf("expected non-zero command result, got %v", err)
	}
	if !errors.Is(err, failureCause) {
		t.Fatalf("command error=%v does not retain batch failure cause", err)
	}
	if !committed {
		t.Fatal("command did not mark a partially published report as committed")
	}
	if manager.calls != 1 {
		t.Fatalf("manager calls=%d, want 1", manager.calls)
	}
	if !equalDownloadPages(manager.request.Pages, []int{1, 3, 4}) || manager.request.Quality != downloader.DownloadQualitySmall || manager.request.UgoiraFormat != downloader.UgoiraFormatAPNG {
		t.Fatalf("download request=%+v", manager.request)
	}
	wantWarning := "warning: artwork 42 (ugoira): using default filename\n"
	if errorOutput.String() != wantWarning {
		t.Fatalf("stderr=%q, want %q", errorOutput.String(), wantWarning)
	}
	if output.Len() != 0 {
		t.Fatalf("stdout=%q, want empty success stream", output.String())
	}
}

type cliDownloadManagerStub struct {
	result  downloader.DownloadBatchResult
	calls   int
	request downloader.DownloadRequest
}

func (cliDownloadManagerStub) SetDownloadPath(string) error { return nil }

func (m *cliDownloadManagerStub) Download(_ context.Context, request downloader.DownloadRequest) (downloader.DownloadBatchResult, error) {
	m.calls++
	m.request = request
	return m.result, nil
}

func equalDownloadPages(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range want {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
