package sdk

import (
	"context"
	"errors"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/atomicfile"
)

// SaveProgress reports incremental progress during a save. Total is the
// expected size in bytes, or zero when unknown; Done is the number of bytes read
// so far.
type SaveProgress struct {
	Total int64
	Done  int64
}

// SaveOptions controls a single-resource atomic save. Path receives the
// completed file, so a partially transferred file never appears there. Progress,
// when non-nil, runs on the reading goroutine; it must not block or perform
// network I/O.
type SaveOptions struct {
	Path     string
	Progress func(SaveProgress)
}

// SavedResource reports where a product save finished and the size it wrote.
// ContentType comes from the product's own response metadata.
type SavedResource struct {
	Path        string
	Size        int64
	ContentType string
}

// WriteResource atomically saves a validated resource stream with progress.
// Product SDK implementations supply their error product/operation labels and
// the known byte total (zero when unknown). The caller owns and closes source.
// Write failures return LocalStateError with only a redacted cause; cancellation
// remains recognizable through errors.Is and leaves the destination unchanged.
func WriteResource(ctx context.Context, product, operation string, source io.Reader, total int64, options SaveOptions) (int64, error) {
	size, err := atomicfile.AtomicWrite(ctx, options.Path, &progressReader{ctx: ctx, src: source, total: total, progress: options.Progress})
	if err == nil {
		return size, nil
	}
	var cause error
	switch {
	case errors.Is(err, context.Canceled):
		cause = context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		cause = context.DeadlineExceeded
	}
	return size, NewError(product, operation, LocalStateError, WithDetail("cannot write resource"), WithCause(cause))
}

type progressReader struct {
	ctx      context.Context
	src      io.Reader
	total    int64
	progress func(SaveProgress)
	done     int64
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 {
		r.done += int64(n)
		if r.progress != nil {
			r.progress(SaveProgress{Total: r.total, Done: r.done})
		}
	}
	// Read may return data and EOF together; cancellation must precede commit.
	if (err == nil || err == io.EOF) && r.ctx.Err() != nil {
		err = r.ctx.Err()
	}
	return n, err
}
