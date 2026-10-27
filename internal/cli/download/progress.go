package download

import (
	"fmt"
	"io"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"golang.org/x/term"
)

// NewProgressRendererForTerminal 只在 stderr 是交互终端时启用。JSON、NDJSON 和
// 重定向场景不注入该回调，因此不会污染任一机器可读或管道输出。
func NewProgressRendererForTerminal(errOut io.Writer) (func(sdk.SaveProgress), bool) {
	file, ok := errOut.(interface{ Fd() uintptr })
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return nil, false
	}
	renderer := NewProgressRenderer(errOut)
	return renderer.Report, true
}

// ProgressRenderer 把一次下载命令的进度更新串行写入终端，避免多批下载交错的进度行。
type ProgressRenderer struct {
	writer   io.Writer
	mu       sync.Mutex
	finished bool
}

// NewProgressRenderer 创建只写一个终端的进度渲染器。
func NewProgressRenderer(writer io.Writer) *ProgressRenderer {
	return &ProgressRenderer{writer: writer}
}

// Report 串行输出一次保存进度；总数为 0 的未知进度不打印。
func (r *ProgressRenderer) Report(progress sdk.SaveProgress) {
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
