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

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/lifecycle"
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
	if _, err := auth.NewHandler(base, store); err != nil {
		return nil, err
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
	return c.Account > 0 && c.Artwork > 0 && c.Page > 0 && (c.Quality == "thumbnail" || c.Quality == "regular" || c.Quality == "original")
}

// Issue scopes access to one account/artwork/page/quality and the issuing OAuth grant.
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
	mac := hmac.New(sha256.New, s.key[:])
	mac.Write([]byte(payload))
	return s.base + "/media/" + payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
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
	if !ok || g.Revoked || g.Resource != s.base+"/mcp" || g.Scope != "mcp" {
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
	mac := hmac.New(sha256.New, s.key[:])
	mac.Write([]byte(payload))
	if !hmac.Equal(sig, mac.Sum(nil)) {
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
	if r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", 405)
		return
	}
	download := r.URL.RawQuery == "download=1"
	if !strings.HasPrefix(r.URL.EscapedPath(), s.Path()) || (r.URL.RawQuery != "" && !download) {
		http.Error(w, "Invalid image request", 400)
		return
	}
	c, err := s.verify(r.Context(), strings.TrimPrefix(r.URL.EscapedPath(), s.Path()))
	if err != nil {
		http.Error(w, "Image authorization expired or invalid", 401)
		return
	}
	if download && c.Quality != "original" {
		http.Error(w, "Invalid image request", 400)
		return
	}
	if s.open == nil && s.reads == nil {
		http.Error(w, "Image service unavailable", 503)
		return
	}

	session, err := s.session(r.Context(), c.Account)
	if session.Lease != nil {
		defer session.Lease.Close()
	}
	if err != nil || session.Lease == nil || session.Lease.Value() == nil {
		http.Error(w, "Image unavailable", 502)
		return
	}
	key := contentKey(c, session, "image")
	if s.reads != nil {
		if s.authorized(r.Context(), c) != nil {
			http.Error(w, "Image authorization expired or invalid", 401)
			return
		}
		if chunks, release, ok := s.reads.cache.load(key); ok {
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
						return
					}
				}
			}
			return
		}
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
	var ref sdk.ResourceRef
	variant := c.Quality
	if variant == "thumbnail" {
		variant = "thumb"
	}
	if artwork.Kind == pixiv.ArtworkKindUgoira {
		if c.Page == 1 && c.Quality != "original" {
			ref = artwork.Cover.Resource.Ref
			if c.Quality == "thumbnail" {
				ref, err = pixiv.ArtworkVariantResource(artwork.Cover.Resource, "square_medium")
			}
		}
	} else {
		for _, page := range artwork.Pages {
			if page.PageIndex+1 == c.Page {
				ref, err = pixiv.ArtworkVariantResource(page.Image.Resource, variant)
				break
			}
		}
	}
	if ref.IsZero() || err != nil {
		http.Error(w, "Image unavailable", 502)
		return
	}
	started := time.Now()
	res, err := session.Lease.Value().OpenResource(r.Context(), sdk.OpenResourceRequest{Ref: ref})
	if err != nil {
		if session.Invalidate != nil {
			session.Invalidate(err)
		}
		http.Error(w, "Image unavailable", 502)
		return
	}
	var closeOnce sync.Once
	var closeErr error
	closeBody := func() error { closeOnce.Do(func() { closeErr = res.Body.Close() }); return closeErr }
	defer closeBody()
	if res.StatusCode != 200 {
		if res.StatusCode == 401 && session.Invalidate != nil {
			session.Invalidate(sdk.NewError("pixiv", "OpenResource", sdk.Unauthorized))
		}
		http.Error(w, "Image unavailable", 502)
		return
	}
	// Sniff before committing headers; do not wait for the complete image.
	prefix := make([]byte, 512)
	n, err := io.ReadFull(res.Body, prefix)
	prefix = prefix[:n]
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		http.Error(w, "Image unavailable", 502)
		return
	}
	mime := http.DetectContentType(prefix)
	switch mime {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp":
	default:
		http.Error(w, "Image unavailable", 502)
		return
	}
	if r.Context().Err() != nil || s.authorized(r.Context(), c) != nil {
		http.Error(w, "Image authorization expired or invalid", 401)
		return
	}
	length := int64(-1)
	if raw := res.Header().Get("Content-Length"); raw != "" {
		if parsed, e := strconv.ParseInt(raw, 10, 64); e == nil && parsed >= 0 {
			length = parsed
		}
	}
	if length >= 0 && length < int64(n) {
		http.Error(w, "Image unavailable", 502)
		return
	}
	imageHeaders(w, c, mime, download)
	if length >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	}
	w.WriteHeader(200)
	if r.Method == "HEAD" {
		return
	}
	var fill *cacheFill
	if s.reads != nil && (length < 0 || length <= s.reads.config.Capacity) {
		fill = s.reads.cache.begin()
		defer fill.abort()
	}
	transfer := time.Now()
	defer s.observe(r.Context(), "image_transfer", transfer)
	write := func(data []byte) error {
		if err := r.Context().Err(); err != nil {
			return err
		}
		if _, e := w.Write(data); e != nil {
			return e
		}
		if fill != nil {
			fill.append(data)
		}
		return nil
	}
	if err := write(prefix); err != nil {
		return
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	s.observe(r.Context(), "image_first_bytes", started)
	total := int64(n)
	buffer := make([]byte, 32*1024)
	for {
		n, e := res.Body.Read(buffer)
		if n > 0 {
			if err := write(buffer[:n]); err != nil {
				return
			}
			total += int64(n)
		}
		if e != nil {
			if e != io.EOF || (length >= 0 && length != total) {
				panic(http.ErrAbortHandler)
			}
			break
		}
	}
	if closeBody() != nil {
		return
	}
	if r.Context().Err() == nil && fill != nil {
		fill.commitValidated(key, s.expiry(c, session), func() bool { return s.canCache(r.Context(), c, session) })
	}
}

func imageHeaders(w http.ResponseWriter, c claims, mime string, download bool) {
	w.Header().Set("Content-Type", mime)
	if download {
		extension := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp", "image/bmp": "bmp"}[mime]
		filename := strconv.FormatInt(c.Artwork, 10) + "_p" + strconv.Itoa(c.Page) + "." + extension
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	}
}
