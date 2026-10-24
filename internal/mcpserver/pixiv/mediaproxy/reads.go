package mediaproxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
)

// Session is an account-owner read lease with an opaque invalidation generation.
type Session struct {
	Lease       *lifecycle.Lease[*pixiv.Client]
	Version     string
	Expires     time.Time
	Invalidate  func(error)
	Invalidated <-chan struct{}
}

// Config is loaded once at server startup. Observe receives fixed stage names only.
// AnimationObservation contains safe phase metrics, never identity or capabilities.
type AnimationObservation struct {
	Stage              string
	Duration           time.Duration
	Bytes              int64
	Failed             bool
	CancellationSource string
}

// DefaultCacheTTL and DefaultConcurrency are the fixed production cache lifetime and
// per-kind upstream read limit; only Capacity remains user-configurable.
const (
	DefaultCacheTTL    = 15 * time.Minute
	DefaultConcurrency = 4
)

type Config struct {
	Capacity         int64
	TTL              time.Duration
	Concurrency      int
	Observe          func(context.Context, string, time.Duration)
	ObserveAnimation func(context.Context, AnimationObservation)
}
type metadataFlight struct {
	done    chan struct{}
	artwork pixiv.Artwork
	err     error
}
type readState struct {
	config                    Config
	cache                     *contentCache
	open                      func(context.Context, int64) (Session, error)
	metadataSlots, imageSlots chan struct{}
	mu                        sync.Mutex
	flights                   map[string]*metadataFlight
}

// Configure installs read sessions and the shared metadata/image budget before serving.
func (s *Service) Configure(config Config, open func(context.Context, int64) (Session, error)) error {
	if config.Capacity < 0 || config.TTL <= 0 || config.Concurrency <= 0 || open == nil {
		return errors.New("invalid gallery read configuration")
	}
	s.reads = &readState{config: config, cache: newContentCache(config.Capacity), open: open, metadataSlots: make(chan struct{}, config.Concurrency), imageSlots: make(chan struct{}, config.Concurrency), flights: map[string]*metadataFlight{}}
	return nil
}
func (s *Service) observe(ctx context.Context, stage string, start time.Time) {
	if s.reads != nil && s.reads.config.Observe != nil {
		s.reads.config.Observe(ctx, stage, time.Since(start))
	}
}
func acquire(ctx context.Context, slots chan struct{}) error {
	if slots == nil {
		return ctx.Err()
	}
	select {
	case slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Service) session(ctx context.Context, account int64) (result Session, err error) {
	start := time.Now()
	defer func() {
		s.observeAnimation(ctx, AnimationObservation{Stage: "session_acquire", Duration: time.Since(start), Failed: err != nil})
	}()
	if s.reads != nil {
		result, err = s.reads.open(ctx, account)
		if err == nil {
			s.trackCacheSession(account, result)
		}
		return result, err
	}
	lease, err := s.open(ctx, account)
	return Session{Lease: lease}, err
}

func (s *Service) trackCacheSession(account int64, session Session) {
	a := s.animations
	if a == nil || session.Invalidated == nil || s.reads.config.Capacity == 0 {
		return
	}
	identity := fmt.Sprintf("%d/%s", account, session.Version)
	a.mu.Lock()
	if _, tracked := a.cacheSessions[identity]; tracked || a.closed || a.ctx.Err() != nil {
		a.mu.Unlock()
		return
	}
	a.cacheSessions[identity] = struct{}{}
	a.sessionWG.Add(1)
	a.mu.Unlock()
	go func() {
		defer a.sessionWG.Done()
		select {
		case <-a.ctx.Done():
		case <-session.Invalidated:
			s.reads.cache.invalidate(func(key string) bool {
				parts := strings.SplitN(key, "/", 5)
				return len(parts) == 5 && parts[2]+"/"+parts[3] == identity
			})
		}
		a.mu.Lock()
		delete(a.cacheSessions, identity)
		a.mu.Unlock()
	}()
}
func (s *Service) expiry(c claims, session Session) time.Time {
	expiry := c.Expires
	if s.reads != nil {
		if ttl := time.Now().Add(s.reads.config.TTL); ttl.Before(expiry) {
			expiry = ttl
		}
	}
	if !session.Expires.IsZero() && session.Expires.Before(expiry) {
		expiry = session.Expires
	}
	return expiry
}

// cache publication and invalidation use the cache mutex, preventing a late fill
// from repopulating content after the invalidation notification was consumed.
func (s *Service) canCache(ctx context.Context, c claims, session Session) bool {
	if s.animations != nil && s.animations.ctx.Err() != nil {
		return false
	}
	if session.Invalidated != nil {
		select {
		case <-session.Invalidated:
			return false
		default:
		}
	}
	return s.authorized(ctx, c) == nil
}
func contentKey(c claims, session Session, kind string) string {
	return fmt.Sprintf("%s/%s/%d/%s/%d/%d/%s", kind, c.Grant, c.Account, session.Version, c.Artwork, c.Page, c.Quality)
}
func (s *Service) metadata(ctx context.Context, c claims, session Session) (pixiv.Artwork, error) {
	if s.authorized(ctx, c) != nil {
		return pixiv.Artwork{}, errCapability
	}
	if session.Lease == nil || session.Lease.Value() == nil {
		return pixiv.Artwork{}, errors.New("missing client")
	}
	query := func() (result pixiv.Artwork, err error) {
		start := time.Now()
		defer func() {
			s.observeAnimation(ctx, AnimationObservation{Stage: "artwork_query", Duration: time.Since(start), Failed: err != nil})
		}()
		if s.reads != nil {
			if err := acquire(ctx, s.reads.metadataSlots); err != nil {
				return pixiv.Artwork{}, err
			}
			defer func() { <-s.reads.metadataSlots }()
		}
		value, err := session.Lease.Value().Artwork(ctx, pixiv.ArtworkRequest{ArtworkID: c.Artwork})
		if err != nil && session.Invalidate != nil {
			session.Invalidate(err)
		}
		return value, err
	}
	if s.reads == nil {
		return query()
	}
	// Metadata is shared by all pages/qualities but never by different grants or sessions.
	c.Page = 0
	c.Quality = ""
	key := contentKey(c, session, "metadata")
	if value, ok := s.cachedArtwork(key); ok {
		s.observe(ctx, "cache_hit", time.Now())
		return value, nil
	}

	state := s.reads
	state.mu.Lock()
	if flight := state.flights[key]; flight != nil {
		state.mu.Unlock()
		select {
		case <-ctx.Done():
			return pixiv.Artwork{}, ctx.Err()
		case <-flight.done:
			if err := s.authorized(ctx, c); err != nil {
				return pixiv.Artwork{}, err
			}
			return flight.artwork, flight.err
		}
	}
	// A fill may have completed between the first lookup and taking the flight lock.
	if value, ok := s.cachedArtwork(key); ok {
		state.mu.Unlock()
		return value, nil
	}
	flight := &metadataFlight{done: make(chan struct{})}
	state.flights[key] = flight
	state.mu.Unlock()
	value, err := query()
	if err == nil {
		if data, e := marshalArtwork(value); e == nil {
			state.cache.putValidated(key, data, s.expiry(c, session), func() bool { return s.canCache(ctx, c, session) })
		}
	}
	state.mu.Lock()
	flight.artwork = value
	flight.err = err
	delete(state.flights, key)
	close(flight.done)
	state.mu.Unlock()
	return value, err
}

// Artwork reauthorizes cached Gallery metadata; mutable bookmark state is never stored.
func (s *Service) Artwork(ctx context.Context, account, artwork int64) (pixiv.Artwork, error) {
	info := sdkauth.TokenInfoFromContext(ctx)
	if info == nil {
		return pixiv.Artwork{}, errCapability
	}
	grant, _ := info.Extra["grant_id"].(string)
	c := claims{Grant: grant, Account: account, Artwork: artwork, Expires: info.Expiration}
	if err := s.authorized(ctx, c); err != nil {
		return pixiv.Artwork{}, err
	}
	session, err := s.session(ctx, account)
	if err != nil {
		return pixiv.Artwork{}, err
	}
	defer session.Lease.Close()
	return s.metadata(ctx, c, session)
}

// ConcurrentReads reports the configured Gallery query worker count.
func (s *Service) ConcurrentReads() int {
	if s == nil || s.reads == nil {
		return 4
	}
	return s.reads.config.Concurrency
}

// HasReadSessions reports whether Configure installed the cached read path.
func (s *Service) HasReadSessions() bool { return s != nil && s.reads != nil }

// cachedResource 是进程内缓存的私有资源格式。它显式保留定位信息；不能嵌入
// sdk.Resource，否则会继承其安全 MarshalJSON 并丢掉 URL。
type cachedResource struct {
	Ref                 *sdk.ResourceRef  `json:"ref,omitempty"`
	URL                 string            `json:"url,omitempty"`
	RequestHeaders      map[string]string `json:"request_headers,omitempty"`
	ExpiresAt           *time.Time        `json:"expires_at,omitempty"`
	RequiresCredentials bool              `json:"requires_credentials,omitempty"`
}

type cachedImage struct {
	Resource cachedResource `json:"resource"`
	Variant  string         `json:"variant"`
	Width    int            `json:"width"`
	Height   int            `json:"height"`
}

type cachedUser struct {
	pixiv.User
	ProfileImage cachedImage `json:"profile_image"`
}

type cachedPage struct {
	pixiv.ArtworkPage
	Image cachedImage `json:"image"`
}

type cachedArtwork struct {
	pixiv.Artwork
	Cover cachedImage  `json:"cover"`
	User  cachedUser   `json:"user"`
	Pages []cachedPage `json:"pages"`
}

func cacheResource(value sdk.Resource) cachedResource {
	out := cachedResource{
		URL:                 value.URL,
		RequestHeaders:      value.RequestHeaders,
		RequiresCredentials: value.RequiresCredentials,
	}
	if !value.Ref.IsZero() {
		ref := value.Ref
		out.Ref = &ref
	}
	if value.ExpiresAt != nil {
		expires := *value.ExpiresAt
		out.ExpiresAt = &expires
	}
	return out
}

func (c cachedResource) resource() sdk.Resource {
	out := sdk.Resource{
		URL:                 c.URL,
		RequestHeaders:      c.RequestHeaders,
		ExpiresAt:           c.ExpiresAt,
		RequiresCredentials: c.RequiresCredentials,
	}
	if c.Ref != nil {
		out.Ref = *c.Ref
	}
	return out
}

func cacheImage(value pixiv.ImageResource) cachedImage {
	return cachedImage{Resource: cacheResource(value.Resource), Variant: value.Variant, Width: value.Width, Height: value.Height}
}

func (c cachedImage) image() pixiv.ImageResource {
	return pixiv.ImageResource{Resource: c.Resource.resource(), Variant: c.Variant, Width: c.Width, Height: c.Height}
}

func (c cachedUser) user() pixiv.User {
	out := c.User
	out.ProfileImage = c.ProfileImage.image()
	return out
}

func (c cachedPage) page() pixiv.ArtworkPage {
	out := c.ArtworkPage
	out.Image = c.Image.image()
	return out
}

func (c cachedArtwork) artwork() pixiv.Artwork {
	out := c.Artwork
	out.Cover = c.Cover.image()
	out.User = c.User.user()
	out.Pages = make([]pixiv.ArtworkPage, len(c.Pages))
	for i, page := range c.Pages {
		out.Pages[i] = page.page()
	}
	return out
}

func marshalArtwork(value pixiv.Artwork) ([]byte, error) {
	pages := make([]cachedPage, len(value.Pages))
	for i, page := range value.Pages {
		pages[i] = cachedPage{ArtworkPage: page, Image: cacheImage(page.Image)}
	}
	wire := cachedArtwork{
		Artwork: value,
		Cover:   cacheImage(value.Cover),
		User:    cachedUser{User: value.User, ProfileImage: cacheImage(value.User.ProfileImage)},
		Pages:   pages,
	}
	return json.Marshal(wire)
}

func (s *Service) cachedArtwork(key string) (pixiv.Artwork, bool) {
	chunks, release, ok := s.reads.cache.load(key)
	defer release()
	if !ok || len(chunks) != 1 {
		return pixiv.Artwork{}, false
	}
	var wire cachedArtwork
	if err := json.Unmarshal(chunks[0], &wire); err != nil {
		return pixiv.Artwork{}, false
	}
	return wire.artwork(), true
}
