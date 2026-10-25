// Package mediaproxy delivers only capability-authorized Pixiv images, never arbitrary URLs.
package mediaproxy

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
)

type Service struct {
	animations *animationState
	reads      *readState
	base       string
	store      auth.Store
	key        [32]byte
	open       func(context.Context, int64) (*lifecycle.Lease[*pixiv.Client], error)
}
type claims struct {
	Grant            string
	Account, Artwork int64
	Page             int
	Quality          string
	Expires          time.Time
}

var errCapability = errors.New("image authorization unavailable")

// New owns a process-local signing key; restarting invalidates issued image URLs.
func New(base string, store auth.Store, open func(context.Context, int64) (*lifecycle.Lease[*pixiv.Client], error)) (*Service, error) {
	if _, err := auth.ParseBaseURL(base); err != nil {
		return nil, err
	}
	if store.Path == "" {
		return nil, errors.New("MCP state path is required")
	}
	s := &Service{base: strings.TrimRight(base, "/"), store: store, open: open}
	if _, err := rand.Read(s.key[:]); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *Service) Origin() string { u, _ := url.Parse(s.base); return u.Scheme + "://" + u.Host }
func (s *Service) Path() string   { u, _ := url.Parse(s.base); return u.EscapedPath() + "/media/" }
func valid(c claims) bool {
	return c.Account > 0 && c.Artwork > 0 && c.Page > 0 && (c.Quality == "thumbnail" || c.Quality == "regular" || c.Quality == "original" || (c.Quality == "avatar" && c.Page == 1))
}

// Issue scopes access to one account/artwork/page/quality and the issuing OAuth grant.
// The internal avatar quality selects only the artwork author's profile image on page 1.
func (s *Service) Issue(ctx context.Context, account, artwork int64, page int, quality string) (string, error) {
	if s == nil {
		return "", errCapability
	}
	info := sdkauth.TokenInfoFromContext(ctx)
	if info == nil {
		return "", errCapability
	}
	grant, _ := info.Extra["grant_id"].(string)
	c := claims{grant, account, artwork, page, quality, info.Expiration}
	if !valid(c) || s.authorized(ctx, c) != nil {
		return "", errCapability
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(b)
	return s.base + "/media/" + payload + "." + base64.RawURLEncoding.EncodeToString(s.signCapability(payload)), nil
}

func (s *Service) signCapability(payload string) []byte {
	mac := hmac.New(sha256.New, s.key[:])
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}
func (s *Service) authorized(ctx context.Context, c claims) error {
	if ctx.Err() != nil || !time.Now().Before(c.Expires) {
		return errCapability
	}
	state, err := s.store.Read(ctx)
	if err != nil {
		return errCapability
	}
	g, ok := state.Grants[c.Grant]
	if !ok || !g.Usable(s.base+"/mcp") {
		return errCapability
	}
	return nil
}
func (s *Service) verify(ctx context.Context, token string) (claims, error) {
	var c claims
	payload, signature, ok := strings.Cut(token, ".")
	if !ok {
		return c, errCapability
	}
	sig, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return c, errCapability
	}
	if !hmac.Equal(sig, s.signCapability(payload)) {
		return c, errCapability
	}
	b, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil || json.Unmarshal(b, &c) != nil || !valid(c) {
		return c, errCapability
	}
	return c, s.authorized(ctx, c)
}

// ServeHTTP intentionally logs neither the path capability nor upstream errors.
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.EscapedPath(), s.Path()+"animation/") {
		s.serveAnimation(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	c, download, ok := s.imageRequest(w, r)
	if !ok {
		return
	}
	session, ok := s.openImageSession(w, r, c)
	if !ok {
		return
	}
	defer session.Lease.Close()
	if s.serveCachedImage(w, r, c, session, download) {
		return
	}
	s.streamImage(w, r, c, session, download)
}

// imageRequest 校验方法、路径、query 与 capability，并返回下载意图。
func (s *Service) imageRequest(w http.ResponseWriter, r *http.Request) (c claims, download, ok bool) {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", 405)
		return c, false, false
	}
	download = r.URL.RawQuery == "download=1"
	if !strings.HasPrefix(r.URL.EscapedPath(), s.Path()) || (r.URL.RawQuery != "" && !download) {
		http.Error(w, "Invalid image request", 400)
		return c, false, false
	}
	c, err := s.verify(r.Context(), strings.TrimPrefix(r.URL.EscapedPath(), s.Path()))
	if err != nil {
		http.Error(w, "Image authorization expired or invalid", 401)
		return c, false, false
	}
	if download && c.Quality != "original" {
		http.Error(w, "Invalid image request", 400)
		return c, false, false
	}
	if s.open == nil && s.reads == nil {
		http.Error(w, "Image service unavailable", 503)
		return c, false, false
	}
	return c, download, true
}

// openImageSession 取得账号持有者的读取快照；调用方负责关闭 Lease。
func (s *Service) openImageSession(w http.ResponseWriter, r *http.Request, c claims) (Session, bool) {
	session, err := s.session(r.Context(), c.Account)
	if err != nil {
		http.Error(w, "Image unavailable", 502)
		return Session{}, false
	}
	return session, true
}

// serveCachedImage 在授权仍然有效时直接交付缓存字节；未命中返回 false。
func (s *Service) serveCachedImage(w http.ResponseWriter, r *http.Request, c claims, session Session, download bool) bool {
	if s.reads == nil {
		return false
	}
	if s.authorized(r.Context(), c) != nil {
		http.Error(w, "Image authorization expired or invalid", 401)
		return true
	}
	chunks, release, ok := s.reads.cache.load(contentKey(c, session, "image"))
	if !ok {
		return false
	}
	defer release()
	s.observe(r.Context(), "cache_hit", time.Now())
	var size int64
	for _, chunk := range chunks {
		size += int64(len(chunk))
	}
	imageHeaders(w, c, http.DetectContentType(chunks[0]), download)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.WriteHeader(200)
	if r.Method == "GET" {
		for _, chunk := range chunks {
			if _, err := w.Write(chunk); err != nil {
				return true
			}
		}
	}
	return true
}

// streamImage 解析作品资源并在上游完成前开始交付；缓存填充与授权复核在传输内完成。
func (s *Service) streamImage(w http.ResponseWriter, r *http.Request, c claims, session Session, download bool) {
	if s.reads != nil {
		if err := acquire(r.Context(), s.reads.imageSlots); err != nil {
			http.Error(w, "Image unavailable", 502)
			return
		}
		defer func() { <-s.reads.imageSlots }()
	}
	artwork, err := s.metadata(r.Context(), c, session)
	if err != nil {
		http.Error(w, "Image unavailable", 502)
		return
	}
	ref, err := imageResource(c, artwork)
	if err != nil {
		http.Error(w, "Image unavailable", 502)
		return
	}
	s.sendImage(w, r, c, session, download, ref)
}

// imageResource 把 capability 的页与质量映射到当前作品的一个资源引用。
func imageResource(c claims, artwork pixiv.Artwork) (sdk.ResourceRef, error) {
	var ref sdk.ResourceRef
	variant := c.Quality
	if variant == "thumbnail" {
		variant = "thumb"
	}
	switch {
	case c.Quality == "avatar":
		ref = artwork.User.ProfileImage.Resource.Ref
	case artwork.Kind == pixiv.ArtworkKindUgoira:
		if c.Page == 1 && c.Quality != "original" {
			ref = artwork.Cover.Resource.Ref
			if c.Quality == "thumbnail" {
				return pixiv.ArtworkVariantResource(artwork.Cover.Resource, "square_medium")
			}
		}
	default:
		for _, page := range artwork.Pages {
			if page.PageIndex+1 == c.Page {
				return pixiv.ArtworkVariantResource(page.Image.Resource, variant)
			}
		}
	}
	if ref.IsZero() {
		return ref, errCapability
	}
	return ref, nil
}

// sendImage 打开上游资源，按嗅探结果提交头部并流式转发，同时按预算填充缓存。
func (s *Service) sendImage(w http.ResponseWriter, r *http.Request, c claims, session Session, download bool, ref sdk.ResourceRef) {
	started := time.Now()
	res, contentType, prefix, length, ok := s.openImage(w, r, c, session, ref)
	if !ok {
		return
	}
	var closeOnce sync.Once
	var closeErr error
	closeBody := func() error { closeOnce.Do(func() { closeErr = res.Body.Close() }); return closeErr }
	defer closeBody()
	imageHeaders(w, c, contentType, download)
	if length >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	}
	w.WriteHeader(200)
	if r.Method == "HEAD" {
		return
	}
	body := imageBody{writer: w, ctx: r.Context(), observe: func(stage string, at time.Time) { s.observe(r.Context(), stage, at) }}
	if s.reads != nil && (length < 0 || length <= s.reads.config.Capacity) {
		body.fill = s.reads.cache.begin()
		defer body.fill.abort()
	}
	if err := s.pipeImage(&body, res.Body, prefix, length, started); err != nil {
		return
	}
	if closeBody() != nil {
		return
	}
	if r.Context().Err() == nil && body.fill != nil {
		body.fill.commitValidated(contentKey(c, session, "image"), s.expiry(c, session), func() bool { return s.canCache(r.Context(), c, session) })
	}
}

// openImage 打开并校验上游资源；只在提交响应头前读取嗅探前缀。
func (s *Service) openImage(w http.ResponseWriter, r *http.Request, c claims, session Session, ref sdk.ResourceRef) (*sdk.ResourceResponse, string, []byte, int64, bool) {
	res, err := session.Lease.Value().OpenResource(r.Context(), sdk.OpenResourceRequest{Ref: ref})
	if err != nil {
		if session.Invalidate != nil {
			session.Invalidate(err)
		}
		http.Error(w, "Image unavailable", 502)
		return nil, "", nil, 0, false
	}
	if res.StatusCode != 200 {
		_ = res.Body.Close()
		if res.StatusCode == 401 && session.Invalidate != nil {
			session.Invalidate(sdk.NewError("pixiv", "OpenResource", sdk.Unauthorized))
		}
		http.Error(w, "Image unavailable", 502)
		return nil, "", nil, 0, false
	}
	// Sniff before committing headers; do not wait for the complete image.
	prefix := make([]byte, 512)
	n, err := io.ReadFull(res.Body, prefix)
	prefix = prefix[:n]
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		_ = res.Body.Close()
		http.Error(w, "Image unavailable", 502)
		return nil, "", nil, 0, false
	}
	contentType := http.DetectContentType(prefix)
	switch contentType {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp":
	default:
		_ = res.Body.Close()
		http.Error(w, "Image unavailable", 502)
		return nil, "", nil, 0, false
	}
	if r.Context().Err() != nil || s.authorized(r.Context(), c) != nil {
		_ = res.Body.Close()
		http.Error(w, "Image authorization expired or invalid", 401)
		return nil, "", nil, 0, false
	}
	length := int64(-1)
	if raw := res.Header().Get("Content-Length"); raw != "" {
		if parsed, e := strconv.ParseInt(raw, 10, 64); e == nil && parsed >= 0 {
			length = parsed
		}
	}
	if length >= 0 && length < int64(n) {
		_ = res.Body.Close()
		http.Error(w, "Image unavailable", 502)
		return nil, "", nil, 0, false
	}
	return res, contentType, prefix, length, true
}

// imageBody 交付已校验的图片字节，并按预算填入缓存。
type imageBody struct {
	writer  http.ResponseWriter
	ctx     context.Context
	fill    *cacheFill
	observe func(stage string, at time.Time)
}

// pipeImage 转发完整图片；写入失败时返回错误，让调用方放弃缓存填充。
func (s *Service) pipeImage(body *imageBody, source io.Reader, prefix []byte, length int64, started time.Time) error {
	transfer := time.Now()
	defer body.observe("image_transfer", transfer)
	if err := body.write(prefix); err != nil {
		return err
	}
	if f, ok := body.writer.(http.Flusher); ok {
		f.Flush()
	}
	body.observe("image_first_bytes", started)
	total := int64(len(prefix))
	buffer := make([]byte, 32*1024)
	for {
		n, err := source.Read(buffer)
		if n > 0 {
			if err := body.write(buffer[:n]); err != nil {
				return err
			}
			total += int64(n)
		}
		if err != nil {
			if err != io.EOF || (length >= 0 && length != total) {
				panic(http.ErrAbortHandler)
			}
			return nil
		}
	}
}

func (b *imageBody) write(data []byte) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	if _, err := b.writer.Write(data); err != nil {
		return err
	}
	if b.fill != nil {
		b.fill.append(data)
	}
	return nil
}

func imageHeaders(w http.ResponseWriter, c claims, mime string, download bool) {
	w.Header().Set("Content-Type", mime)
	if download {
		extension := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp", "image/bmp": "bmp"}[mime]
		filename := strconv.FormatInt(c.Artwork, 10) + "_p" + strconv.Itoa(c.Page) + "." + extension
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	}
}
