package mcpserver

import (
	"context"
	"errors"
	"io"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RunStdio 运行 JSON-RPC 服务；输入断开或进程退出时取消在途工具，等它们清理完再返回，
// 之后调用方才能释放 SDK 资源。
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

// Read 在任何读错误（包括 io.EOF）时取消服务，让工具停止处理已无法交付的请求。
func (r stdioReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if err != nil {
		r.cancel(err)
	}
	return n, err
}

type stdioWriter struct{ io.Writer }

// Close 不关闭调用方提供的输出流。
func (stdioWriter) Close() error { return nil }
