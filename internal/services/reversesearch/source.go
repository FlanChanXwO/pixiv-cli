package reversesearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

// SourceLoaderOptions 是载荷加载器的构造依赖。TempDir 为空时使用系统临时目录；
// HTTPClient 由 URL source 使用，加载器不会修改调用方对象。
type SourceLoaderOptions struct {
	TempDir    string
	HTTPClient *http.Client
}

// SourceLoader 将一个 source 固化为可重复打开的私有快照。
type SourceLoader interface {
	Load(context.Context, string) (*Snapshot, error)
}

// Loader 实现文件与 HTTP(S) source 的单次流式载入。
type Loader struct {
	tempDir    string
	httpClient *http.Client
	openFile   func(string) (*os.File, error)
}

func NewSourceLoader(options SourceLoaderOptions) *Loader {
	return &Loader{tempDir: options.TempDir, httpClient: options.HTTPClient, openFile: os.Open}
}

// Snapshot 是 provider 共享的不可变载荷。调用方必须在所有 reader 关闭后 Close。
type Snapshot struct {
	mu     sync.Mutex
	path   string
	kind   SourceKind
	sha256 string
	size   int64
	closed bool
}

func (s *Snapshot) Kind() SourceKind { return s.kind }

func (s *Snapshot) SHA256() string { return s.sha256 }

func (s *Snapshot) Size() int64 { return s.size }

// Open 从快照起点返回一个独立 reader，不暴露临时文件路径。
func (s *Snapshot) Open() (io.ReadCloser, error) {
	if s == nil {
		return nil, NewError(CodeSnapshotFailed, "image snapshot is unavailable", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, NewError(CodeSnapshotFailed, "image snapshot is closed", nil)
	}
	reader, err := os.Open(s.path)
	if err != nil {
		return nil, NewError(CodeSnapshotFailed, "could not open image snapshot", err)
	}
	return reader, nil
}

// Close 删除私有快照；重复调用安全。
func (s *Snapshot) Close() error {
	if s == nil {
		return nil
	}
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

func (l *Loader) Load(ctx context.Context, source string) (*Snapshot, error) {
	if ctx == nil {
		return nil, NewError(CodeInvalidRequest, "reverse search context is required", nil)
	}
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
	input, err := l.openSourceFile(source)
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

func (l *Loader) openSourceFile(path string) (*os.File, error) {
	if l.openFile == nil {
		return os.Open(path)
	}
	return l.openFile(path)
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
	baseClient := l.httpClient
	if baseClient == nil {
		baseClient = http.DefaultClient
	}
	client := *baseClient
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
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, NewError(CodeSourceReadFailed, "could not fetch image source", err)
	}
	defer response.Body.Close()
	if response.Request == nil || response.Request.URL == nil {
		return nil, NewError(CodeInvalidSource, "image source URL is invalid", nil)
	}
	if err := validateSourceURL(response.Request.URL.String()); err != nil {
		return nil, err
	}
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
	temporary, err := os.CreateTemp(l.tempDir, "pixiv-reverse-search-*.tmp")
	if err != nil {
		return nil, NewError(CodeSnapshotFailed, "could not create image snapshot", err)
	}
	path := temporary.Name()
	defer func() {
		if err != nil {
			_ = temporary.Close()
			_ = os.Remove(path)
		}
	}()
	if chmodErr := temporary.Chmod(0o600); chmodErr != nil {
		return nil, NewError(CodeSnapshotFailed, "could not secure image snapshot", chmodErr)
	}

	digest := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(temporary, digest), contextReader{ctx: ctx, reader: input})
	if copyErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, NewError(CodeSourceReadFailed, "could not read image source", copyErr)
	}
	if closeErr := temporary.Close(); closeErr != nil {
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
	return r.reader.Read(buffer)
}
