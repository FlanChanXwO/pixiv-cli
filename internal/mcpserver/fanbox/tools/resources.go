package tools

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk"
	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/FlanChanXwO/pixiv-cli/internal/media/filename"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (t fanboxTools) registerOpenResource(server *mcp.Server) {
	toolrun.ProtectInput(server, "fanbox_open_resource", func() *mcp.CallToolResult {
		out := openResourceOut{Error: "invalid_request"}
		result := outputs.Result(true, out.Error)
		result.StructuredContent = out
		return result
	})
	toolrun.AddTool(t.calls, server, &mcp.Tool{Name: "fanbox_open_resource", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Read a FANBOX resource by opaque ref. GET returns real image or embedded binary content and safe metadata; HEAD returns metadata only. Does not save files on the server."}, func(ctx context.Context, request *mcp.CallToolRequest, input openResourceIn) (*mcp.CallToolResult, openResourceOut, error) {
		return t.openResourceHandler(ctx, input)
	})
}

type openResourceIn struct {
	Ref    string `json:"ref" jsonschema:"opaque FANBOX media resource reference"`
	Method string `json:"method,omitempty" jsonschema:"GET or HEAD; default GET"`
}

type openResourceOut struct {
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

func (t fanboxTools) openResourceHandler(ctx context.Context, input openResourceIn) (*mcp.CallToolResult, openResourceOut, error) {
	out := openResourceOut{}
	ref, err := legacysdk.ParseResourceRef(input.Ref)
	if err != nil {
		out.Error = "invalid_ref"
		return outputs.Result(true, out.Error), out, nil
	}
	product, err := legacysdk.ResourceRefProduct(ref)
	if err != nil || product != "fanbox" {
		out.Error = "invalid_ref"
		return outputs.Result(true, out.Error), out, nil
	}
	method := legacysdk.ResourceMethod(strings.ToUpper(input.Method))
	if method == "" {
		method = legacysdk.ResourceMethodGet
	}
	if method != legacysdk.ResourceMethodGet && method != legacysdk.ResourceMethodHead {
		out.Error = "invalid_method"
		return outputs.Result(true, out.Error), out, nil
	}
	var content []mcp.Content
	err = lifecycle.Run(ctx, t.openClient, func(ctx context.Context, client *fanbox.Client, _ *lifecycle.Attempt) error {
		response, err := openResource(ctx, client, ref, string(method))
		if err != nil {
			return err
		}
		out.Filename = filename.Sanitize(response.Filename)
		out.StatusCode = response.StatusCode
		out.ContentType = response.Header.Get("Content-Type")
		out.ContentLength, _ = strconv.ParseInt(response.Header.Get("Content-Length"), 10, 64)
		if response.StatusCode != http.StatusOK {
			return errors.Join(&sdk.Error{Operation: "OpenResource", Reason: sdk.UpstreamError, HTTPStatus: response.StatusCode}, response.Body.Close())
		}
		if method == legacysdk.ResourceMethodHead {
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
			return &sdk.Error{Operation: "OpenResource", Reason: sdk.MalformedUpstreamResponse}
		}
		// 图片按实际字节嗅探；其他附件保留有效的专用 MIME，例如 ZIP 容器里的 Office 文件。
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
		}
		if errors.Is(err, context.Canceled) {
			out.Error = "canceled"
		} else if errors.Is(err, context.DeadlineExceeded) {
			out.Error = "deadline_exceeded"
		}
	}
	out.Complete = err == nil
	result := outputs.Result(!out.Complete, out.Error)
	if len(content) > 0 {
		result.Content = content
	}
	return result, out, nil
}

// isAPNG 要求 acTL 出现在首个 IDAT 之前，因此逐个遍历 chunk；压缩载荷里的同名字节
// 不参与判断。
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

// openResource 只解码稳定身份，再通过绑定的新 SDK 对象取得当前地址。
func openResource(ctx context.Context, client *fanbox.Client, ref legacysdk.ResourceRef, method string) (*fanbox.MediaResponse, error) {
	payload, err := legacysdk.ResourceRefPayload(ref)
	if err != nil {
		return nil, err
	}
	var identity record.FanboxResourceIdentity
	if json.Unmarshal(payload, &identity) != nil {
		return nil, &sdk.Error{Reason: sdk.InvalidArgument, Operation: "OpenResource", Message: "invalid resource reference"}
	}
	request := fanbox.OpenRequest{Method: method}
	switch identity.Role {
	case record.FanboxCreatorIcon, record.FanboxCreatorCover:
		creator, err := client.Creator(identity.Creator).Get(ctx)
		if err != nil {
			return nil, err
		}
		image := creator.Icon
		if identity.Role == record.FanboxCreatorCover {
			image = creator.Cover
		}
		return image.Open(ctx, request)
	case record.FanboxPostImage, record.FanboxPostFile:
		post, err := client.Post(identity.Post).Get(ctx)
		if err != nil {
			return nil, err
		}
		if post.Body != nil {
			if identity.Role == record.FanboxPostImage {
				for _, image := range post.Body.Images {
					if image.ID == identity.Asset {
						return image.Open(ctx, request)
					}
				}
			} else {
				for _, file := range post.Body.Files {
					if file.ID == identity.Asset {
						return file.Open(ctx, request)
					}
				}
			}
		}
		return nil, &sdk.Error{Reason: sdk.MalformedUpstreamResponse, Operation: "OpenResource", Message: "post attachment is unavailable"}
	default:
		return nil, &sdk.Error{Reason: sdk.InvalidArgument, Operation: "OpenResource", Message: "resource kind is unsupported"}
	}
}
