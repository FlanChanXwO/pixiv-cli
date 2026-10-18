// Package openresource 实现 fanbox_open_resource tool。
package openresource

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader/filename"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 fanbox_open_resource。
func Register(app *runtime.App, server *mcp.Server) {
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, request)
			call, ok := request.(*mcp.CallToolRequest)
			if !ok || call.Params.Name != "fanbox_open_resource" {
				return result, err
			}
			failed, ok := result.(*mcp.CallToolResult)
			if !ok || !failed.IsError || failed.StructuredContent != nil {
				return result, err
			}
			out := Out{Error: "invalid_request"}
			safe := runtime.Result(out, true, out.Error)
			safe.StructuredContent = out
			return safe, nil
		}
	})
	runtime.AddTool(app, server, &mcp.Tool{Name: "fanbox_open_resource", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Read a FANBOX resource by opaque ref. GET returns real image or embedded binary content and safe metadata; HEAD returns metadata only. Does not save files on the server."}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		return handle(ctx, app, input)
	})
}

type In struct {
	Ref    string `json:"ref" jsonschema:"opaque FANBOX media resource reference"`
	Method string `json:"method,omitempty" jsonschema:"GET or HEAD; default GET"`
}

type Out struct {
	Filename      string `json:"filename,omitempty"`
	Ref           string `json:"ref"`
	StatusCode    int    `json:"status_code"`
	ContentType   string `json:"content_type,omitempty"`
	ContentLength int64  `json:"content_length,omitempty"`
	Size          int    `json:"size"`
	ContentIndex  *int   `json:"content_index,omitempty"`
	Delivered     bool   `json:"delivered"`
	Complete      bool   `json:"complete"`
	Error         string `json:"error,omitempty"`
}

func handle(ctx context.Context, app *runtime.App, input In) (*mcp.CallToolResult, Out, error) {
	out := Out{}
	ref, err := sdk.ParseResourceRef(input.Ref)
	if err != nil {
		out.Error = "invalid_ref"
		return runtime.Result(out, true, out.Error), out, nil
	}
	product, err := sdk.ResourceRefProduct(ref)
	if err != nil || product != "fanbox" {
		out.Error = "invalid_ref"
		return runtime.Result(out, true, out.Error), out, nil
	}
	method := sdk.ResourceMethod(strings.ToUpper(input.Method))
	if method == "" {
		method = sdk.ResourceMethodGet
	}
	if method != sdk.ResourceMethodGet && method != sdk.ResourceMethodHead {
		out.Error = "invalid_method"
		return runtime.Result(out, true, out.Error), out, nil
	}
	var content []mcp.Content
	err = lifecycle.Run(ctx, app.OpenClient, func(ctx context.Context, client *fanbox.Client, _ *lifecycle.Attempt) error {
		response, err := client.OpenResource(ctx, sdk.OpenResourceRequest{Ref: ref, Method: method})
		if err != nil {
			return err
		}
		out.Filename = filename.Sanitize(response.Filename)
		out.StatusCode = response.StatusCode
		out.ContentType = response.ContentType()
		out.ContentLength = response.ContentLength()
		if response.StatusCode != http.StatusOK {
			return errors.Join(sdk.NewError("fanbox", "OpenResource", sdk.UpstreamError, sdk.WithHTTPStatus(response.StatusCode)), response.Body.Close())
		}
		if method == sdk.ResourceMethodHead {
			out.Ref = ref.String()
			return response.Body.Close()
		}
		data, readErr := io.ReadAll(response.Body)
		if err := errors.Join(readErr, response.Body.Close()); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		detected := http.DetectContentType(data)
		declared, _, _ := mime.ParseMediaType(out.ContentType)
		if strings.HasPrefix(declared, "image/") && !strings.HasPrefix(detected, "image/") {
			return sdk.NewError("fanbox", "OpenResource", sdk.MalformedUpstreamResponse)
		}
		// 图片按实际bytes识别；其它附件保留有效的专用MIME（如ZIP容器中的Office文件）。
		out.ContentType = declared
		if strings.HasPrefix(detected, "image/") || declared == "" {
			out.ContentType = detected
		}
		if detected == "image/png" && isAPNG(data) {
			out.ContentType = "image/apng"
		}
		if strings.HasPrefix(out.ContentType, "image/") && out.ContentType != "image/gif" && out.ContentType != "image/apng" {
			content = []mcp.Content{&mcp.ImageContent{MIMEType: out.ContentType, Data: data}}
		} else {
			content = []mcp.Content{&mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: fmt.Sprintf("urn:sha256:%x", sha256.Sum256(data)), MIMEType: out.ContentType, Blob: data}}}
		}
		out.Ref = ref.String()
		out.Size = len(data)
		out.ContentIndex = new(0)
		out.Delivered = true
		return nil
	})
	if err != nil {
		out.Error = "resource_read_failed"
		var typed *sdk.Error
		if errors.As(err, &typed) {
			out.Error = string(typed.Reason)
			if out.StatusCode == 0 {
				out.StatusCode = typed.HTTPStatus
			}
		}
		if errors.Is(err, context.Canceled) {
			out.Error = "canceled"
		} else if errors.Is(err, context.DeadlineExceeded) {
			out.Error = "deadline_exceeded"
		}
	}
	out.Complete = err == nil
	result := runtime.Result(out, !out.Complete, out.Error)
	if len(content) > 0 {
		result.Content = content
	}
	return result, out, nil
}

// APNG的acTL必须位于首个IDAT之前；遍历chunk而非搜索压缩payload中的同名字节。
func isAPNG(data []byte) bool {
	for offset := 8; len(data)-offset >= 12; {
		size := uint64(binary.BigEndian.Uint32(data[offset : offset+4]))
		if size > uint64(len(data)-offset-12) {
			return false
		}
		switch string(data[offset+4 : offset+8]) {
		case "acTL":
			return size == 8
		case "IDAT", "IEND":
			return false
		}
		offset += int(size) + 12
	}
	return false
}
