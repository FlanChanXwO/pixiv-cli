package download

import (
	"context"
	"errors"
	"testing"

	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
)

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
