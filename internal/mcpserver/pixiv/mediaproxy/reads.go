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

// Session 表示一次账号只读租约。Version 标识凭据版本；Invalidated 关闭后该租约的缓存
// 作废，但只有调用 ConfigureAnimations 后才有人监听并执行失效。调用方负责关闭 Lease。
type Session struct {
	Lease       *lifecycle.Lease[*pixiv.Client]
	Version     string
	Expires     time.Time
	Invalidate  func(error)
	Invalidated <-chan struct{}
}

// AnimationObservation 记录动图阶段的耗时与字节数，不含身份、URL 或 capability。
type AnimationObservation struct {
	Stage              string
	Duration           time.Duration
	Bytes              int64
	Failed             bool
	CancellationSource string
}

// cacheTTL 与 readConcurrency 是固定的生产取值，只有 Capacity 来自用户配置。
const (
	cacheTTL        = 15 * time.Minute
	readConcurrency = 4
)

// Config 在服务启动时加载一次；观察回调只会收到固定的阶段名。
type Config struct {
	Capacity         int64
	Observe          func(context.Context, string, time.Duration)
	ObserveAnimation func(context.Context, AnimationObservation)
}
type metadataFlight struct {
	done    chan struct{}
	artwork *artworkMetadata
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

// Configure 必须在开始服务前调用，安装读会话与元数据、图片共享的字节预算。
func (s *Service) Configure(config Config, open func(context.Context, int64) (Session, error)) error {
	if config.Capacity < 0 || open == nil {
		return errors.New("invalid gallery read configuration")
	}
	s.reads = &readState{config: config, cache: newContentCache(config.Capacity), open: open, metadataSlots: make(chan struct{}, readConcurrency), imageSlots: make(chan struct{}, readConcurrency), flights: map[string]*metadataFlight{}}
	return nil
}
func (s *Service) observe(ctx context.Context, stage string, start time.Time) {
	if s.reads != nil && s.reads.config.Observe != nil {
		s.reads.config.Observe(ctx, stage, time.Since(start))
	}
}
func acquire(ctx context.Context, slots chan struct{}) error {
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
		if ttl := time.Now().Add(cacheTTL); ttl.Before(expiry) {
			expiry = ttl
		}
	}
	if !session.Expires.IsZero() && session.Expires.Before(expiry) {
		expiry = session.Expires
	}
	return expiry
}

// canCache 判断内容是否仍可写入缓存：服务已关闭、会话失效或当前 grant 不再有效时都返回
// false，避免迟到的填充把已撤销的内容写回。
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
func (s *Service) metadata(ctx context.Context, c claims, session Session) (*artworkMetadata, error) {
	if s.authorized(ctx, c) != nil {
		return nil, errCapability
	}
	query := func() (result *artworkMetadata, err error) {
		start := time.Now()
		defer func() {
			s.observeAnimation(ctx, AnimationObservation{Stage: "artwork_query", Duration: time.Since(start), Failed: err != nil})
		}()
		if s.reads != nil {
			if err := acquire(ctx, s.reads.metadataSlots); err != nil {
				return nil, err
			}
			defer func() { <-s.reads.metadataSlots }()
		}
		value, err := session.Lease.Value().Artwork(c.Artwork).Get(ctx)
		if err != nil && session.Invalidate != nil {
			session.Invalidate(err)
		}
		if err != nil {
			return nil, err
		}
		snapshot := &artworkMetadata{Artwork: value, PageImages: map[int]map[string]pixiv.Image{}}
		for _, page := range value.Pages {
			if value.ContentType != pixiv.ContentTypeUgoira || page.Original.Variant() == "original" {
				snapshot.OriginalPages = append(snapshot.OriginalPages, page.PageIndex)
			}
		}
		if value.ContentType == pixiv.ContentTypeUgoira {
			snapshot.Thumbnail, err = value.Cover.Quality(pixiv.ImageQualitySquareMedium)
			if err != nil && sdk.ReasonOf(err) != sdk.InvalidArgument {
				return nil, err
			}
		}
		if value.ContentType != pixiv.ContentTypeUgoira {
			for _, page := range value.Pages {
				variants := map[string]pixiv.Image{"original": page.Original}
				for _, quality := range []string{"regular", "thumb"} {
					image, variantErr := page.Original.Quality(pixiv.ImageQuality(quality))
					if variantErr == nil {
						variants[quality] = image
					} else if sdk.ReasonOf(variantErr) != sdk.InvalidArgument {
						return nil, variantErr
					}
				}
				snapshot.PageImages[page.PageIndex] = variants
			}
		}
		return snapshot, nil
	}
	if s.reads == nil {
		return query()
	}
	// 元数据在页与画质之间共享，但不同 grant 或会话之间从不共享。
	c.Page = 0
	c.Quality = ""
	key := contentKey(c, session, "metadata")
	if value, ok, err := s.cachedArtwork(key); ok || err != nil {
		if err == nil {
			s.observe(ctx, "cache_hit", time.Now())
		}
		return value, err
	}

	state := s.reads
	state.mu.Lock()
	if flight := state.flights[key]; flight != nil {
		state.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-flight.done:
			if err := s.authorized(ctx, c); err != nil {
				return nil, err
			}
			return flight.artwork, flight.err
		}
	}
	// 第一次查询与取得 flight 锁之间，填充可能已经完成。
	if value, ok, err := s.cachedArtwork(key); ok || err != nil {
		state.mu.Unlock()
		return value, err
	}
	flight := &metadataFlight{done: make(chan struct{})}
	state.flights[key] = flight
	state.mu.Unlock()
	value, err := query()
	if err == nil {
		data, encodeErr := marshalArtwork(value)
		if encodeErr != nil {
			err = fmt.Errorf("encode Gallery metadata cache: %w", encodeErr)
		} else {
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

// Artwork 复用 Gallery 元数据缓存前重新校验当前 grant；缓存里不保存会变的收藏状态。
func (s *Service) Artwork(ctx context.Context, account, artwork int64) (*pixiv.Artwork, []int, error) {
	info := sdkauth.TokenInfoFromContext(ctx)
	if info == nil {
		return nil, nil, errCapability
	}
	grant, _ := info.Extra["grant_id"].(string)
	c := claims{Grant: grant, Account: account, Artwork: artwork, Expires: info.Expiration}
	if err := s.authorized(ctx, c); err != nil {
		return nil, nil, err
	}
	session, err := s.session(ctx, account)
	if err != nil {
		return nil, nil, err
	}
	defer session.Lease.Close()
	metadata, err := s.metadata(ctx, c, session)
	if err != nil {
		return nil, nil, err
	}
	return metadata.Artwork, metadata.OriginalPages, nil
}

// ConcurrentReads 返回固定的 Gallery 查询并发数。
func (s *Service) ConcurrentReads() int { return readConcurrency }

// artworkMetadata 把选中的封面缩略图与作品数据一起计入序列化缓存预算。
type artworkMetadata struct {
	OriginalPages []int                          `json:"original_pages"`
	Artwork       *pixiv.Artwork                 `json:"artwork"`
	Thumbnail     pixiv.Image                    `json:"thumbnail"`
	PageImages    map[int]map[string]pixiv.Image `json:"page_images"`
}

// 收藏状态随实时工具读取；账号绑定在资源打开处重新建立。
func marshalArtwork(value *artworkMetadata) ([]byte, error) {
	snapshot := *value.Artwork
	snapshot.IsBookmarked = nil
	return json.Marshal(&artworkMetadata{Artwork: &snapshot, Thumbnail: value.Thumbnail, PageImages: value.PageImages, OriginalPages: value.OriginalPages})
}

func (s *Service) cachedArtwork(key string) (*artworkMetadata, bool, error) {
	chunks, release, ok := s.reads.cache.load(key)
	defer release()
	if !ok {
		return nil, false, nil
	}
	var value artworkMetadata
	if err := json.Unmarshal(chunks[0], &value); err != nil {
		return nil, false, fmt.Errorf("decode Gallery metadata cache: %w", err)
	}
	return &value, true, nil
}
