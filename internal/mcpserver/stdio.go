// stdio.go 在输入断开时取消 JSON-RPC 调用，并保留输出流的所有权。

package mcpserver

import (
	"context"
	"errors"
	"io"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RunStdio serves JSON-RPC, cancels active tools on input loss or process shutdown,
// and waits for their cleanup before the caller releases SDK resources.
func RunStdio(ctx context.Context, server *mcp.Server, in io.Reader, out io.Writer) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	bindLifecycle(ctx, server)
	reader, ok := in.(io.ReadCloser)
	if !ok {
		reader = io.NopCloser(in)
	}
	err := server.Run(ctx, &mcp.IOTransport{Reader: stdioReader{reader, cancel}, Writer: stdioWriter{out}})
	if errors.Is(context.Cause(ctx), io.EOF) {
		return nil
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return err
}

type stdioReader struct {
	io.ReadCloser
	cancel context.CancelCauseFunc
}

func (r stdioReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if err != nil {
		r.cancel(err)
	}
	return n, err
}

type stdioWriter struct{ io.Writer }

func (stdioWriter) Close() error { return nil }
