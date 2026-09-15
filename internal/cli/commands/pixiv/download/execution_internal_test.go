package download

import (
	"context"
	"errors"
	"testing"

	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
)

var errWarningWriter = errors.New("warning writer failed")

type failingWarningWriter struct{}

func (failingWarningWriter) Write([]byte) (int, error) {
	return 0, errWarningWriter
}

func TestDownloadAttemptWithWarningsPreservesBusinessError(t *testing.T) {
	businessErr := errors.New("business failure")
	committed, err := downloadAttemptWithWarnings(failingWarningWriter{}, downloader.DownloadReport{
		Failures: []downloader.DownloadFailure{{Message: businessErr.Error(), Cause: businessErr}},
		Warnings: []downloader.DownloadWarning{{Message: "warning"}},
	}, nil)

	if committed {
		t.Fatal("downloadAttemptWithWarnings committed = true, want false")
	}
	if !errors.Is(err, businessErr) {
		t.Fatalf("downloadAttemptWithWarnings error = %v, want business error", err)
	}
	if !errors.Is(err, errWarningWriter) {
		t.Fatalf("downloadAttemptWithWarnings error = %v, want warning writer error", err)
	}
}

// TestDownloadAttemptResultPreservesCommittedOnOperationError 锁定 action-record
// 与普通参数路径一致的账号池提交边界：已有文件时 operation error 不能触发重放。
func TestDownloadAttemptResultPreservesCommittedOnOperationError(t *testing.T) {
	cause := context.Canceled
	committed, err := downloadAttemptResult(downloader.DownloadReport{
		Items: []downloader.DownloadedArtwork{{
			IllustID: 42,
			Files:    []downloader.DownloadedFile{{Path: "published.png"}},
		}},
	}, cause)

	if !committed {
		t.Fatal("downloadAttemptResult committed = false, want true for published file")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("downloadAttemptResult error = %v, want %v", err, cause)
	}
}
