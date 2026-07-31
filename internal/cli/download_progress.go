package cli

import (
	"fmt"
	"io"
	"sync"

	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"golang.org/x/term"
)

// newDownloadProgressRenderer 只在 stderr 是交互终端时启用。JSON、NDJSON 和
// 重定向场景不注入该回调，因此不会污染任一机器可读或管道输出。
func newDownloadProgressRenderer(errOut io.Writer) (*downloadProgressRenderer, bool) {
	file, ok := errOut.(interface{ Fd() uintptr })
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return nil, false
	}
	return &downloadProgressRenderer{writer: errOut}, true
}

type downloadProgressRenderer struct {
	writer   io.Writer
	mu       sync.Mutex
	finished bool
}

func (r *downloadProgressRenderer) Report(progress sdk.DownloadProgress) {
	if !progress.TotalBytesKnown {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished {
		return
	}
	percent := int64(100)
	if progress.TotalBytes > 0 {
		percent = progress.TotalBytesTransferred * 100 / progress.TotalBytes
	}
	_, _ = fmt.Fprintf(r.writer, "\rDownloading %d/%d bytes (%d%%)", progress.TotalBytesTransferred, progress.TotalBytes, percent)
	if progress.CompletedResources == progress.TotalResources {
		_, _ = fmt.Fprintln(r.writer)
		r.finished = true
	}
}
