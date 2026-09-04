package download_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/download"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportWarningsWritesToErrorOutput(t *testing.T) {
	var errOut bytes.Buffer
	report := downloader.DownloadReport{
		Warnings: []downloader.DownloadWarning{{
			IllustID: 42,
			Type:     "ugoira",
			Message:  "ugoira filename template failed; using default filename",
		}},
	}

	require.NoError(t, download.ReportWarnings(&errOut, report))
	assert.Equal(t, "warning: artwork 42 (ugoira): ugoira filename template failed; using default filename\n", errOut.String())
}

func TestReportErrorRetainsTypedRateLimitCause(t *testing.T) {
	cause := sdk.NewError("pixiv", "download", sdk.RateLimited, sdk.WithRetry(sdk.RetryAdvice{Safe: true, HasAfter: true}))
	report := downloader.DownloadReport{
		Committed: true,
		Failures:  []downloader.DownloadFailure{{Message: cause.Error(), Cause: cause}},
	}

	err := download.ReportError(report)

	var typed *sdk.Error
	require.ErrorAs(t, err, &typed)
	assert.Same(t, cause, typed)
	assert.Contains(t, err.Error(), "download completed with 1 failures")
}

func TestReportErrorFallsBackToMessageAndCount(t *testing.T) {
	assert.NoError(t, download.ReportError(downloader.DownloadReport{}))

	err := download.ReportError(downloader.DownloadReport{Failures: []downloader.DownloadFailure{{Message: "resource rejected"}}})
	require.Error(t, err)
	assert.EqualError(t, err, "download completed with 1 failures: resource rejected")

	err = download.ReportError(downloader.DownloadReport{Failures: []downloader.DownloadFailure{{}, {}}})
	require.Error(t, err)
	assert.EqualError(t, err, "download completed with 2 failures")
}

// TestReportCommittedMarksPartialMultiPagePublication 固定账号池的重放边界：任何
// 已原子发布的常规文件都禁止重放，即使报告本身没有设置 Committed。
func TestReportCommittedMarksPartialMultiPagePublication(t *testing.T) {
	assert.False(t, download.ReportCommitted(downloader.DownloadReport{}))
	assert.True(t, download.ReportCommitted(downloader.DownloadReport{Committed: true}))
	assert.True(t, download.ReportCommitted(downloader.DownloadReport{
		Items: []downloader.DownloadedArtwork{{IllustID: 42, Files: []downloader.DownloadedFile{{Path: "one.png"}}}},
	}))
	assert.False(t, download.ReportCommitted(downloader.DownloadReport{
		Items:    []downloader.DownloadedArtwork{{IllustID: 42}},
		Failures: []downloader.DownloadFailure{{Message: "resource rejected", Cause: errors.New("boom")}},
	}))
}
