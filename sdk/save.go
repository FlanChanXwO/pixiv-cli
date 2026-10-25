// Atomic resource saving, progress reporting, and cancellation for product SDKs.

package sdk

import (
	"context"
	"errors"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/atomicfile"
)

// SaveProgress reports incremental progress while a resource is being saved.
// Total is the expected size in bytes when known, otherwise zero; Done is the
// number of bytes read from the resource stream so far.
type SaveProgress struct {
	Total int64
	Done  int64
}

// SaveOptions configures a single-resource atomic save. SaveResource writes to
// an atomic destination under Path, so a partially transferred file never
// appears at the final path. Progress, when non-nil, is called as bytes are
// read from the stream; it must not block or perform network I/O.
type SaveOptions struct {
	Path     string
	Progress func(SaveProgress)
}

// SavedResource reports the result of SaveResource. ContentType is the media
// type reported by the product's save operation.
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
