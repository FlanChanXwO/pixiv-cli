package pixiv

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/protocol"
	internalresource "github.com/FlanChanXwO/pixiv-cli/internal/pixiv/resource"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
)

// ResourcePolicy 声明除 Pixiv 官方资源域名外，调用方显式信任的镜像。
type ResourcePolicy struct {
	Mirrors []ResourceMirrorPolicy `json:"mirrors,omitempty"`
}

// ResourceMirrorPolicy 以精确 host 与绝对路径前缀限制一个镜像。
type ResourceMirrorPolicy struct {
	Host         string   `json:"host"`
	PathPrefixes []string `json:"path_prefixes"`
}

// ResourceRef 是可持久化的资源引用；使用前 OpenResource 会再次验证。
type ResourceRef struct {
	URL string `json:"url"`
}

// OpenResourceRequest 描述 range 或条件资源请求。
type OpenResourceRequest struct {
	Ref             ResourceRef
	Range           string
	IfNoneMatch     string
	IfModifiedSince string
}

// ResourceResponse 暴露可代理的状态、白名单 header 与未预读响应流。
type ResourceResponse struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
}

type compiledMirror struct {
	host     string
	prefixes []string
}

type resourcePolicy struct{ mirrors []compiledMirror }

func compileResourcePolicy(policy ResourcePolicy) (resourcePolicy, error) {
	result := resourcePolicy{mirrors: make([]compiledMirror, 0, len(policy.Mirrors))}
	for _, mirror := range policy.Mirrors {
		host := strings.ToLower(strings.TrimSpace(mirror.Host))
		if !validPolicyHost(host) || len(mirror.PathPrefixes) == 0 {
			return resourcePolicy{}, invalidResourceError(OperationParseResourceRef, "resource mirror policy is invalid")
		}
		compiled := compiledMirror{host: host, prefixes: make([]string, len(mirror.PathPrefixes))}
		for i, prefix := range mirror.PathPrefixes {
			if !validPathPrefix(prefix) {
				return resourcePolicy{}, invalidResourceError(OperationParseResourceRef, "resource mirror path prefix is invalid")
			}
			compiled.prefixes[i] = prefix
		}
		result.mirrors = append(result.mirrors, compiled)
	}
	return result, nil
}

func validPolicyHost(host string) bool {
	if host == "" || strings.ContainsAny(host, "/\\@?# ") || hasControl(host) {
		return false
	}
	parsed, err := url.Parse("https://" + host + "/")
	if err != nil || parsed.Host != host || parsed.Hostname() == "" || parsed.User != nil {
		return false
	}
	port := parsed.Port()
	if port == "" {
		return !strings.Contains(host, ":") || net.ParseIP(parsed.Hostname()) != nil
	}
	number, err := strconv.Atoi(port)
	return err == nil && number > 0 && number <= 65535
}

func validPathPrefix(prefix string) bool {
	if prefix == "/" || !strings.HasPrefix(prefix, "/") || strings.ContainsAny(prefix, "\\%?#") || hasControl(prefix) {
		return false
	}
	return path.Clean(prefix) == prefix ||
		(strings.HasSuffix(prefix, "/") && path.Clean(prefix) == strings.TrimSuffix(prefix, "/"))
}

// ParseResourceRef 根据 Client 的资源策略解析并验证 URL。
func (c *Client) ParseResourceRef(rawURL string) (out ResourceRef, err error) {
	started := time.Now()
	defer func() { c.delegatedOperationLog(OperationParseResourceRef, started, err, 0, 0) }()
	if c.defaults != nil {
		scoped, err := c.defaults.resourceSnapshot(OperationParseResourceRef)
		if err != nil {
			c.operationLog(OperationParseResourceRef, started, err, 0, 0)
			return ResourceRef{}, err
		}
		return scoped.ParseResourceRef(rawURL)
	}
	parsed, err := c.resourcePolicy.validate(rawURL, OperationParseResourceRef)
	if err != nil {
		return ResourceRef{}, err
	}
	return ResourceRef{URL: parsed.String()}, nil
}

// OpenResource 打开经 policy 验证的 Pixiv 资源；成功响应的 Body 由调用方关闭。
func (c *Client) OpenResource(ctx context.Context, request OpenResourceRequest) (out *ResourceResponse, err error) {
	started := time.Now()
	defer func() { c.delegatedOperationLog(OperationOpenResource, started, err, 0, 0) }()
	if c.defaults != nil {
		scoped, err := c.defaults.resourceSnapshot(OperationOpenResource)
		if err != nil {
			c.operationLog(OperationOpenResource, started, err, 0, 0)
			return nil, err
		}
		return scoped.OpenResource(ctx, request)
	}
	for _, value := range []string{request.Range, request.IfNoneMatch, request.IfModifiedSince} {
		if hasControl(value) {
			return nil, invalidResourceError(OperationOpenResource, "resource request header is invalid")
		}
	}
	if _, err := c.resourcePolicy.validate(request.Ref.URL, OperationOpenResource); err != nil {
		return nil, err
	}
	headers := make(http.Header)
	setOptionalHeader(headers, "Range", request.Range)
	setOptionalHeader(headers, "If-None-Match", request.IfNoneMatch)
	setOptionalHeader(headers, "If-Modified-Since", request.IfModifiedSince)
	headers.Set("Accept-Encoding", "identity")
	response, err := c.resource.Open(ctx, internalresource.OpenRequest{
		URL:            request.Ref.URL,
		Header:         headers,
		DisableCookies: true,
		Validate: func(rawURL string) error {
			_, err := c.resourcePolicy.validate(rawURL, OperationOpenResource)
			if err == nil {
				return nil
			}
			// redirect validator 的公开 *Error 不能穿过 resource adapter；仅保留
			// 稳定的 policy 分类，避免 URL 在 net/url.Error 中回流。
			if errors.Is(err, ErrForbidden) {
				return protocol.Forbidden()
			}
			return protocol.UpstreamRejected()
		},
	})
	if err != nil {
		var failure protocol.Failure
		if errors.As(err, &failure) && failure.Kind == protocol.FailureForbidden {
			return nil, forbiddenResourceError(OperationOpenResource)
		}
		return nil, mapResourceTransportError(err, OperationOpenResource)
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent && response.StatusCode != http.StatusNotModified {
		_ = response.Body.Close()
		code, retryable := codeForHTTPStatus(response.StatusCode, OperationOpenResource)
		return nil, newError(code, OperationOpenResource, BackendResource, retryable, response.StatusCode, 0, errors.New("resource upstream rejected the request"))
	}
	return &ResourceResponse{StatusCode: response.StatusCode, Header: filteredResourceHeaders(response.Header), Body: response.Body}, nil
}

// Download 将完整资源流式写入同目录临时文件，并在成功后原子替换目标。
func (c *Client) Download(ctx context.Context, ref ResourceRef, destinationPath string) (err error) {
	started := time.Now()
	defer func() { c.delegatedOperationLog(OperationDownload, started, err, 0, 0) }()
	if c.defaults != nil {
		scoped, err := c.defaults.resourceSnapshot(OperationDownload)
		if err != nil {
			c.operationLog(OperationDownload, started, err, 0, 0)
			return err
		}
		return scoped.Download(ctx, ref, destinationPath)
	}
	if strings.TrimSpace(destinationPath) == "" {
		return invalidResourceError(OperationDownload, "destination path is invalid")
	}
	dir := filepath.Dir(destinationPath)
	tmp, err := os.CreateTemp(dir, ".pixiv-download-*")
	if err != nil {
		return invalidResourceError(OperationDownload, "cannot create destination temporary file")
	}
	tmpPath := tmp.Name()
	keepTemp := true
	defer func() {
		if keepTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	response, err := c.OpenResource(ctx, OpenResourceRequest{Ref: ref})
	if err != nil {
		_ = tmp.Close()
		return resourceErrorForOperation(err, OperationDownload)
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		_ = tmp.Close()
		return newError(CodeUpstreamError, OperationDownload, BackendResource, true, response.StatusCode, 0, errors.New("resource was not a complete response"))
	}
	destination := &resourceDestinationWriter{writer: tmp}
	_, copyErr := io.Copy(destination, response.Body)
	bodyCloseErr := response.Body.Close()
	contextErr := ctx.Err()
	tmpCloseErr := tmp.Close()
	if contextErr != nil {
		return mapResourceTransportError(contextErr, OperationDownload)
	}
	if copyErr != nil {
		return classifyResourceCopyError(copyErr, destination.err)
	}
	if bodyCloseErr != nil {
		return newError(CodeUpstreamUnavailable, OperationDownload, BackendResource, true, 0, 0, errors.New("resource stream failed"))
	}
	if tmpCloseErr != nil {
		return invalidResourceError(OperationDownload, "destination temporary file close failed")
	}
	if err := files.ReplaceFile(tmpPath, destinationPath); err != nil {
		return invalidResourceError(OperationDownload, "destination replacement failed")
	}
	keepTemp = false
	return nil
}

type resourceDestinationWriter struct {
	writer io.Writer
	err    error
}

func (w *resourceDestinationWriter) Write(payload []byte) (int, error) {
	written, err := w.writer.Write(payload)
	if err != nil {
		w.err = err
	} else if written != len(payload) {
		w.err = io.ErrShortWrite
	}
	return written, err
}

func classifyResourceCopyError(_ error, destinationErr error) error {
	if destinationErr != nil {
		return invalidResourceError(OperationDownload, "destination write failed")
	}
	return newError(CodeUpstreamUnavailable, OperationDownload, BackendResource, true, 0, 0, errors.New("resource stream failed"))
}

func resourceErrorForOperation(err error, operation Operation) error {
	var typed *Error
	if !errors.As(err, &typed) {
		return mapResourceTransportError(err, operation)
	}
	mapped := newError(typed.Code, operation, typed.Backend, typed.Retryable, typed.UpstreamStatus, 0, errors.Unwrap(typed))
	mapped.TransportKind = typed.TransportKind
	return mapped
}

func setOptionalHeader(header http.Header, key, value string) {
	if value != "" {
		header.Set(key, value)
	}
}

func hasControl(value string) bool {
	for _, char := range []byte(value) {
		if char < 0x20 || char == 0x7f {
			return true
		}
	}
	return false
}

func filteredResourceHeaders(source http.Header) http.Header {
	result := make(http.Header)
	for key, values := range source {
		canonical := http.CanonicalHeaderKey(key)
		if !allowedResourceHeader(canonical) {
			continue
		}
		result[canonical] = append([]string(nil), values...)
	}
	return result
}

func allowedResourceHeader(header string) bool {
	switch header {
	case "Content-Type", "Content-Length", "Cache-Control", "Etag", "Last-Modified",
		"Expires", "Age", "Accept-Ranges", "Content-Range", "Content-Encoding", "Vary":
		return true
	default:
		return false
	}
}

func mapResourceTransportError(err error, operation Operation) error {
	var typed *Error
	if errors.As(err, &typed) {
		return typed
	}
	return mapAdapterFailure(err, operation, BackendResource, 0, 0)
}

func (p resourcePolicy) validate(rawURL string, operation Operation) (*url.URL, error) {
	if rawURL == "" || strings.Contains(rawURL, "\\") {
		return nil, invalidResourceError(operation, "resource url is malformed")
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Opaque != "" || u.Host == "" || u.Path == "" {
		return nil, invalidResourceError(operation, "resource url is malformed")
	}
	if u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.RawFragment != "" {
		return nil, forbiddenResourceError(operation)
	}
	if _, err := url.ParseQuery(u.RawQuery); err != nil {
		return nil, invalidResourceError(operation, "resource query is malformed")
	}
	decodedPath, err := url.PathUnescape(u.EscapedPath())
	if err != nil || decodedPath == "" || unsafeResourcePath(decodedPath) {
		return nil, forbiddenResourceError(operation)
	}
	host := strings.ToLower(u.Host)
	if host == "i.pximg.net" && u.Port() == "" && allowedPixivPath(decodedPath) {
		return u, nil
	}
	for _, mirror := range p.mirrors {
		if host != mirror.host {
			continue
		}
		for _, prefix := range mirror.prefixes {
			if matchesPathPrefix(decodedPath, prefix) {
				return u, nil
			}
		}
	}
	return nil, forbiddenResourceError(operation)
}

func matchesPathPrefix(resourcePath, prefix string) bool {
	if strings.HasSuffix(prefix, "/") {
		return strings.HasPrefix(resourcePath, prefix)
	}
	return resourcePath == prefix || strings.HasPrefix(resourcePath, prefix+"/")
}

func unsafeResourcePath(decodedPath string) bool {
	current := decodedPath
	for {
		if strings.Contains(current, "\\") || hasControl(current) || hasTraversal(current) {
			return true
		}
		next, err := url.PathUnescape(current)
		if err != nil {
			return true
		}
		if next == current {
			return false
		}
		current = next
	}
}

func hasTraversal(decodedPath string) bool {
	for _, segment := range strings.Split(decodedPath, "/") {
		if segment == "." || segment == ".." {
			return true
		}
	}
	return path.Clean(decodedPath) != decodedPath
}

func allowedPixivPath(decodedPath string) bool {
	if strings.HasPrefix(decodedPath, "/img-original/") ||
		strings.HasPrefix(decodedPath, "/img-master/") ||
		strings.HasPrefix(decodedPath, "/img-zip-ugoira/") {
		return true
	}
	return strings.HasPrefix(decodedPath, "/c/") &&
		(strings.Contains(decodedPath, "/img-master/") || strings.Contains(decodedPath, "/img-original/"))
}

func invalidResourceError(operation Operation, message string) *Error {
	return newError(CodeInvalidArgument, operation, "", false, 0, 0, errors.New(message))
}

func forbiddenResourceError(operation Operation) *Error {
	return newError(CodeForbidden, operation, "", false, 0, 0, errors.New("resource url is not allowed"))
}
