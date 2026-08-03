package cli

import (
	"fmt"
	"io"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"golang.org/x/term"
)

// newDownloadProgressRenderer 只在 stderr 是交互终端时启用。JSON、NDJSON 和
// 重定向场景不注入该回调，因此不会污染任一机器可读或管道输出。
func newDownloadProgressRenderer(errOut io.Writer) (func(sdk.SaveProgress), bool) {
	file, ok := errOut.(interface{ Fd() uintptr })
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return nil, false
	}
	renderer := &downloadProgressRenderer{writer: errOut}
	return renderer.Report, true
}

type downloadProgressRenderer struct {
	writer   io.Writer
	mu       sync.Mutex
	finished bool
}

func (r *downloadProgressRenderer) Report(progress sdk.SaveProgress) {
	if progress.Total <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished {
		return
	}
	percent := int64(100)
	if progress.Total > 0 {
		percent = progress.Done * 100 / progress.Total
	}
	_, _ = fmt.Fprintf(r.writer, "\rDownloading %d/%d bytes (%d%%)", progress.Done, progress.Total, percent)
	if progress.Done >= progress.Total {
		_, _ = fmt.Fprintln(r.writer)
		r.finished = true
	}
}
