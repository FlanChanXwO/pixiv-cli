// Package ascii2d searches uploaded image bytes using color and feature matching.
//
// [Search] owns one upload and its result pages. [Options] supplies connection
// settings; [Result] keeps each requested mode's matches and failure separately.
// Cloudflare clearance is cached internally; local configuration is never read.
// Clearance is shared for identical connection settings and HTTPClient identity.
// Site cookies remain local to each call. The last active search closes its shared
// transport and solver session; reusable clearance remains until expiry or rejection.
package ascii2d

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

const (
	origin          = "https://ascii2d.net"
	homeURL         = origin + "/"
	uploadPath      = "/search/file"
	searchPath      = "/search/"
	operationSearch = "ascii2d.Search"
	// The service accepts at most 10 MB per JPEG, PNG, or WEBP upload.
	maxImageBytes = 10 * 1024 * 1024
	mediaJPEG     = "image/jpeg"
	mediaPNG      = "image/png"
	mediaWEBP     = "image/webp"
)

// Mode selects result pages from a single image upload.
type Mode string

const (
	// Color selects the color-similarity page.
	Color Mode = "color"
	// BOVW selects the feature-similarity page.
	BOVW Mode = "bovw"
	// Both selects both pages with one upload.
	Both Mode = "both"
)

// Options configures image bytes, matching, and independent native/solver routes.
type Options struct {
	Image                []byte       `json:"-"` // JPEG, PNG, or WEBP, at most 10 MB; unchanged until Search returns.
	Mode                 Mode         // Empty selects Color.
	HTTPClient           *http.Client `json:"-"` // Optional caller-owned transport; its Jar is bypassed.
	ProxyURL             string
	UserAgent            string // Empty selects the Chrome TLS profile's default agent.
	FlareSolverrURL      string // Empty disables recovery.
	FlareSolverrProxyURL string // Solver browser route; control requests are direct.
}

// Result preserves the independent outcomes of requested pages, including partial failures.
// Unrequested pages are nil; an upload failure leaves both pages nil.
type Result struct {
	Color *Page `json:"color,omitempty"`
	BOVW  *Page `json:"bovw,omitempty"`
}

// Page retains ordered matches and its request error. Err is also returned by Search.
type Page struct {
	Matches []Match `json:"matches"`
	Err     error   `json:"-"`
}

// Match retains one result's original links. ArtworkID is zero without a canonical Pixiv artwork link.
type Match struct {
	Rank         int      `json:"rank"`
	IndexName    string   `json:"index_name"`
	Title        string   `json:"title,omitempty"`
	Author       string   `json:"author,omitempty"`
	ArtworkID    int64    `json:"artwork_id,omitempty"`
	ExternalURLs []string `json:"external_urls,omitempty"`
}

// Search uploads Image once and fetches the selected pages. Results remain
// available when another page fails; errors use sdk.Error and preserve cancellation.
func Search(ctx context.Context, options Options) (result Result, err error) {
	if err := ctx.Err(); err != nil {
		return Result{}, failure(sdk.UpstreamUnavailable, "ascii2d search canceled", err)
	}
	if options.Mode == "" {
		options.Mode = Color
	}
	if options.Mode != Color && options.Mode != BOVW && options.Mode != Both {
		return Result{}, failure(sdk.InvalidArgument, "invalid ascii2d mode", nil)
	}
	mediaType := http.DetectContentType(options.Image)
	if len(options.Image) > maxImageBytes || (mediaType != mediaJPEG && mediaType != mediaPNG && mediaType != mediaWEBP) {
		return Result{}, failure(sdk.InvalidArgument, "ascii2d requires a JPEG, PNG, or WEBP image up to 10 MB", nil)
	}
	if options.UserAgent == "" {
		options.UserAgent = defaultUserAgent
	}
	if strings.ContainsAny(options.UserAgent, "\r\n\x00") {
		return Result{}, failure(sdk.InvalidArgument, "invalid ascii2d User-Agent", nil)
	}
	if options.HTTPClient != nil && options.HTTPClient.Transport == nil {
		return Result{}, failure(sdk.InvalidArgument, "injected HTTP client requires an explicit transport", nil)
	}
	// 1. Acquire shared transport/clearance while keeping CSRF and site cookies local.
	entry, browser, err := acquireClearance(options)
	if err != nil {
		return Result{}, err
	}
	defer func() { err = errors.Join(err, entry.release(ctx)) }()
	session, err := newSession(options, browser, options.UserAgent)
	if err != nil {
		return Result{}, err
	}
	// 2. Upload once, with one isolated replay after a verified challenge.
	hash, target, err := session.upload(ctx, options.Image, mediaType)
	if sdk.ReasonOf(err) == sdk.ChallengeRequired && options.FlareSolverrURL != "" {
		state, solveErr := entry.solve(ctx, browser, target)
		if solveErr != nil {
			return Result{}, solveErr
		}
		session, err = newSession(options, browser, state.UserAgent)
		if err != nil {
			entry.invalidate(browser, state)
			return Result{}, err
		}
		session.httpClient.Jar.SetCookies(session.endpoint, []*http.Cookie{{Name: clearanceCookie, Value: state.Cookie.Value(), Path: "/"}})
		hash, _, err = session.upload(ctx, options.Image, mediaType)
		if sdk.ReasonOf(err) == sdk.ChallengeRequired {
			entry.invalidate(browser, state)
		}
	}
	if err != nil {
		return Result{}, err
	}
	// 3. Fetch selected pages independently so a failed page preserves its sibling.
	var pages sync.WaitGroup
	if options.Mode == Color || options.Mode == Both {
		result.Color = &Page{}
		pages.Go(func() { result.Color.Matches, result.Color.Err = session.search(ctx, Color, hash) })
	}
	if options.Mode == BOVW || options.Mode == Both {
		result.BOVW = &Page{}
		pages.Go(func() { result.BOVW.Matches, result.BOVW.Err = session.search(ctx, BOVW, hash) })
	}
	pages.Wait() // Join both page requests before releasing their shared transport.
	if result.Color != nil {
		err = result.Color.Err
	}
	if result.BOVW != nil {
		err = errors.Join(err, result.BOVW.Err)
	}
	return result, err
}

func failure(reason sdk.Reason, message string, cause error) *sdk.Error {
	return &sdk.Error{Reason: reason, Operation: operationSearch, Message: message, Err: cause}
}
