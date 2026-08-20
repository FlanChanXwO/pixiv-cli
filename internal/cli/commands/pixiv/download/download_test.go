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
