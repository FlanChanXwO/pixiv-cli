package reversesearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
)

// Loader 实现文件与 HTTP(S) source 的单次流式载入。
type Loader struct {
	httpClient *http.Client
}

// NewSourceLoader 使用组合根提供的 HTTP client；加载器不会修改调用方对象。
func NewSourceLoader(client *http.Client) *Loader {
	return &Loader{httpClient: client}
}

// Snapshot 表示 provider 共享的不可变载荷。调用方必须在所有 reader 关闭后 Close。
type Snapshot struct {
	mu     sync.Mutex
	path   string
	kind   SourceKind
	sha256 string
	size   int64
	closed bool
}

// Kind 返回快照来自本地文件还是 HTTP(S) URL。
func (s *Snapshot) Kind() SourceKind { return s.kind }

// SHA256 返回快照内容的十六进制摘要，只作为身份标识，不提供完整性校验。
func (s *Snapshot) SHA256() string { return s.sha256 }

// Size 返回快照的字节数。
func (s *Snapshot) Size() int64 { return s.size }

// Open 从快照起点返回响应 ctx 取消的独立 reader，不暴露临时文件路径。
func (s *Snapshot) Open(ctx context.Context) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, NewError(CodeSnapshotFailed, "image snapshot is closed", nil)
	}
	reader, err := os.Open(s.path)
	if err != nil {
		return nil, NewError(CodeSnapshotFailed, "could not open image snapshot", err)
	}
	return struct {
		io.Reader
		io.Closer
	}{contextReader{ctx: ctx, reader: reader}, reader}, nil
}

// Close 删除私有快照；重复调用安全。
func (s *Snapshot) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return NewError(CodeSnapshotFailed, "could not remove image snapshot", err)
	}
	s.closed = true
	return nil
}

// Load 把文件或 HTTP(S) source 复制成私有快照；返回的快照由调用方 Close。路径不存在、
// 不是普通文件或上游响应失败都返回带稳定分类的错误。
func (l *Loader) Load(ctx context.Context, source string) (*Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if hasHTTPScheme(source) {
		return l.loadURL(ctx, source)
	}
	info, err := os.Stat(source)
	if err != nil {
		if parsed, parseErr := url.Parse(source); parseErr == nil && parsed.Scheme != "" {
			return nil, NewError(CodeInvalidSource, "image source must use HTTP or HTTPS", nil)
		}
		return nil, NewError(CodeSourceReadFailed, "could not read image source", err)
	}
	if !info.Mode().IsRegular() {
		return nil, NewError(CodeSourceNotRegularFile, "image source must be a regular file", nil)
	}
	input, err := os.Open(source)
	if err != nil {
		return nil, NewError(CodeSourceReadFailed, "could not read image source", err)
	}
	defer input.Close()
	info, err = input.Stat()
	if err != nil {
		return nil, NewError(CodeSourceReadFailed, "could not inspect image source", err)
	}
	if !info.Mode().IsRegular() {
		return nil, NewError(CodeSourceNotRegularFile, "image source must be a regular file", nil)
	}
	return l.copySnapshot(ctx, SourceKindFile, input)
}

func hasHTTPScheme(source string) bool {
	lower := strings.ToLower(source)
	return strings.HasPrefix(lower, "http:") || strings.HasPrefix(lower, "https:")
}

func (l *Loader) loadURL(ctx context.Context, source string) (*Snapshot, error) {
	if err := validateSourceURL(source); err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, NewError(CodeInvalidSource, "image source URL is invalid", err)
	}
	client := *l.httpClient
	originalRedirect := client.CheckRedirect
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if originalRedirect != nil {
			if redirectErr := originalRedirect(next, via); redirectErr != nil {
				return redirectErr
			}
		} else if len(via) >= 10 {
			// 保持 net/http 的既有默认重定向限制；这不是反向搜图新增的重试策略。
			return errors.New("stopped after 10 redirects")
		}
		return validateSourceURL(next.URL.String())
	}
	response, err := client.Do(request)
	if err != nil {
		logging.From(ctx).LogAttrs(ctx, slog.LevelDebug, "Request failed", logging.TransportAttrs(err)...)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, NewError(CodeSourceReadFailed, "could not fetch image source", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, NewError(CodeSourceHTTPStatus, "image source returned an unsuccessful HTTP status", nil)
	}
	return l.copySnapshot(ctx, SourceKindURL, response.Body)
}

func validateSourceURL(source string) error {
	parsed, err := url.Parse(source)
	if err != nil || parsed.Host == "" || parsed.Scheme == "" {
		return NewError(CodeInvalidSource, "image source URL is invalid", err)
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return NewError(CodeInvalidSource, "image source must use HTTP or HTTPS", nil)
	}
	if parsed.User != nil {
		return NewError(CodeInvalidSource, "image source URL must not contain user information", nil)
	}
	return nil
}

func (l *Loader) copySnapshot(ctx context.Context, kind SourceKind, input io.Reader) (_ *Snapshot, err error) {
	temporary, err := os.CreateTemp("", "pixiv-reverse-search-*.tmp")
	if err != nil {
		return nil, NewError(CodeSnapshotFailed, "could not create image snapshot", err)
	}
	path := temporary.Name()
	closed := false
	defer func() {
		if err != nil {
			if !closed {
				if closeErr := temporary.Close(); closeErr != nil {
					err = errors.Join(err, NewError(CodeSnapshotFailed, "could not close image snapshot", closeErr))
				}
			}
			if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				err = errors.Join(err, NewError(CodeSnapshotFailed, "could not remove image snapshot", removeErr))
			}
		}
	}()
	if chmodErr := temporary.Chmod(0o600); chmodErr != nil {
		return nil, NewError(CodeSnapshotFailed, "could not secure image snapshot", chmodErr)
	}

	digest := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(temporary, digest), contextReader{ctx: ctx, reader: input})
	if copyErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			// 保持取消原有的分类和消息，同时保留读取原因。
			return nil, NewError(CodeUnknown, ctxErr.Error(), copyErr)
		}
		return nil, NewError(CodeSourceReadFailed, "could not read image source", copyErr)
	}
	closeErr := temporary.Close()
	closed = true
	if closeErr != nil {
		return nil, NewError(CodeSnapshotFailed, "could not finalize image snapshot", closeErr)
	}
	return &Snapshot{
		path:   path,
		kind:   kind,
		sha256: hex.EncodeToString(digest.Sum(nil)),
		size:   size,
	}, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	read, err := r.reader.Read(buffer)
	if ctxErr := r.ctx.Err(); ctxErr != nil {
		if err == io.EOF {
			err = nil
		}
		err = errors.Join(ctxErr, err)
	}
	return read, err
}
