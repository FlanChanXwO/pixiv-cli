package application

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-mcp-server/internal/config"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/download"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/storage/auth"
)

type ArtworkClient interface {
	SearchIllust(context.Context, string, string, string, string, int) (*pixiv.IllustList, error)
	IllustDetail(context.Context, int64) (*pixiv.IllustDetail, error)
	IllustRanking(context.Context, string, string, int) (*pixiv.IllustList, error)
	IllustRecommended(context.Context, int) (*pixiv.IllustList, error)
}

type DownloadClient interface {
	download.PixivClient
}

type ClientBundle struct {
	Auth     AuthenticatedPixivClient
	Artwork  ArtworkClient
	Download DownloadClient
}

type PixivClientFactory func(config.RuntimeConfig) (ClientBundle, error)

type ClientResolver struct {
	Auth            AuthRepository
	LoadRuntime     func() (config.RuntimeConfig, error)
	RefreshTokenEnv func() string
	NewClient       PixivClientFactory
}

type ClientRequest struct {
	Profile                  string
	RefreshToken             string
	DownloadPathOverride     *string
	FilenameTemplateOverride *string
	JSONOverride             *bool
	NeedsAuth                bool
}

type ClientSession struct {
	Client  ClientBundle
	Config  config.RuntimeConfig
	JSONOut bool
}

func (r ClientResolver) Resolve(ctx context.Context, req ClientRequest) (ClientSession, error) {
	cfg, err := r.runtime()
	if err != nil {
		return ClientSession{}, err
	}
	if req.DownloadPathOverride != nil {
		cfg.DownloadPath = *req.DownloadPathOverride
	}
	if req.FilenameTemplateOverride != nil {
		cfg.FilenameTemplate = *req.FilenameTemplateOverride
	}
	jsonOut := cfg.OutputJSON
	if req.JSONOverride != nil {
		jsonOut = *req.JSONOverride
	}
	store, err := r.authStore()
	if err != nil {
		return ClientSession{}, err
	}
	refreshToken, err := ResolveRefreshToken(store, req.Profile, req.RefreshToken, r.refreshTokenFromEnv)
	if err != nil {
		return ClientSession{}, err
	}
	cfg.RefreshToken = refreshToken
	if req.NeedsAuth && cfg.RefreshToken == "" {
		return ClientSession{}, errors.New("missing refresh token; use PIXIV_REFRESH_TOKEN or pixiv auth add/login")
	}
	if r.NewClient == nil {
		return ClientSession{}, errors.New("pixiv client factory is not configured")
	}
	client, err := r.NewClient(cfg)
	if err != nil {
		return ClientSession{}, err
	}
	if req.NeedsAuth {
		if client.Auth == nil {
			return ClientSession{}, errors.New("pixiv auth client is not configured")
		}
		if err := client.Auth.Refresh(ctx); err != nil {
			return ClientSession{}, err
		}
	}
	return ClientSession{Client: client, Config: cfg, JSONOut: jsonOut}, nil
}

func (r ClientResolver) authStore() (auth.AuthStore, error) {
	if r.Auth == nil {
		return auth.AuthStore{}, errors.New("auth repository is not configured")
	}
	return r.Auth.Load()
}

func (r ClientResolver) runtime() (config.RuntimeConfig, error) {
	if r.LoadRuntime == nil {
		return config.RuntimeConfig{}, errors.New("runtime config loader is not configured")
	}
	return r.LoadRuntime()
}

func (r ClientResolver) refreshTokenFromEnv() string {
	if r.RefreshTokenEnv != nil {
		return r.RefreshTokenEnv()
	}
	return ""
}

type ArtworkService struct {
	Resolver ClientResolver
}

type SearchRequest struct {
	Client   ClientRequest
	Word     string
	Target   string
	Sort     string
	Duration string
	Offset   int
}

type RankingRequest struct {
	Client ClientRequest
	Mode   string
	Date   string
	Offset int
}

type RecommendedRequest struct {
	Client ClientRequest
	Offset int
}

func (s ArtworkService) Search(ctx context.Context, req SearchRequest) (*pixiv.IllustList, bool, error) {
	session, err := s.Resolver.Resolve(ctx, req.Client)
	if err != nil {
		return nil, false, err
	}
	if session.Client.Artwork == nil {
		return nil, false, errors.New("pixiv artwork client is not configured")
	}
	result, err := session.Client.Artwork.SearchIllust(ctx, req.Word, req.Target, req.Sort, req.Duration, req.Offset)
	return result, session.JSONOut, err
}

func (s ArtworkService) Detail(ctx context.Context, clientReq ClientRequest, id int64) (*pixiv.IllustDetail, bool, error) {
	session, err := s.Resolver.Resolve(ctx, clientReq)
	if err != nil {
		return nil, false, err
	}
	if session.Client.Artwork == nil {
		return nil, false, errors.New("pixiv artwork client is not configured")
	}
	result, err := session.Client.Artwork.IllustDetail(ctx, id)
	return result, session.JSONOut, err
}

func (s ArtworkService) Ranking(ctx context.Context, req RankingRequest) (*pixiv.IllustList, bool, error) {
	session, err := s.Resolver.Resolve(ctx, req.Client)
	if err != nil {
		return nil, false, err
	}
	if session.Client.Artwork == nil {
		return nil, false, errors.New("pixiv artwork client is not configured")
	}
	result, err := session.Client.Artwork.IllustRanking(ctx, req.Mode, req.Date, req.Offset)
	return result, session.JSONOut, err
}

func (s ArtworkService) Recommended(ctx context.Context, req RecommendedRequest) (*pixiv.IllustList, bool, error) {
	clientReq := req.Client
	clientReq.NeedsAuth = true
	session, err := s.Resolver.Resolve(ctx, clientReq)
	if err != nil {
		return nil, false, err
	}
	if session.Client.Artwork == nil {
		return nil, false, errors.New("pixiv artwork client is not configured")
	}
	result, err := session.Client.Artwork.IllustRecommended(ctx, req.Offset)
	return result, session.JSONOut, err
}

type DownloadService struct {
	Resolver      ClientResolver
	NewDownloader DownloadFactory
}

type Downloader interface {
	Download(context.Context, []int64) ([]download.DownloadedArtwork, error)
}

type DownloadFactory func(DownloadClient, config.RuntimeConfig) Downloader

func (s DownloadService) Download(ctx context.Context, clientReq ClientRequest, ids []int64) ([]download.DownloadedArtwork, bool, error) {
	session, err := s.Resolver.Resolve(ctx, clientReq)
	if err != nil {
		return nil, false, err
	}
	if s.NewDownloader == nil {
		return nil, false, errors.New("download factory is not configured")
	}
	if session.Client.Download == nil {
		return nil, false, errors.New("pixiv download client is not configured")
	}
	downloader := s.NewDownloader(session.Client.Download, session.Config)
	artworks, err := downloader.Download(ctx, ids)
	return artworks, session.JSONOut, err
}

type Services struct {
	Account  AccountService
	Config   ConfigService
	Artwork  ArtworkService
	Download DownloadService
	Login    LoginService
}
