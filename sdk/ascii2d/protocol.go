package ascii2d

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/textproto"
	"net/url"
	"regexp"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/internal/cloudflare"
)

const (
	clearanceCookie   = "cf_clearance"
	headerContentType = "Content-Type"
	headerDisposition = "Content-Disposition"
	headerLocation    = "Location"
	headerMitigated   = "Cf-Mitigated"
	paramToken        = "authenticity_token"
	paramFile         = "file"
	homePath          = "/"
	defaultHTTPPort   = "80"
	defaultHTTPSPort  = "443"
	maxRedirects      = 10 // Preserve net/http's default redirect limit.
)

var (
	uploadHashPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
	errCrossOrigin    = errors.New("ascii2d redirected outside its origin")
)

type searchSession struct {
	httpClient     *http.Client
	endpoint       *url.URL
	userAgent      string
	hints          clientHints
	acceptEncoding string
}

func newSession(options Options, browser *cloudflare.Client, agent string) (*searchSession, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, failure(sdk.LocalStateError, "could not create ascii2d cookie jar", err)
	}
	client := http.Client{Transport: browser}
	if options.HTTPClient != nil {
		client = *options.HTTPClient
	}
	client.Jar = jar
	prior := client.CheckRedirect
	endpoint, _ := url.Parse(origin)
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if !sameOrigin(endpoint, next.URL) {
			return errCrossOrigin
		}
		if prior != nil {
			return prior(next, via)
		}
		if len(via) >= maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}
		return nil
	}
	session := &searchSession{httpClient: &client, endpoint: endpoint, userAgent: agent, hints: parseClientHints(agent)}
	if options.HTTPClient == nil {
		// Injected transports negotiate and decode their own compression.
		session.acceptEncoding = browserEncoding
	}
	return session, nil
}

func (s *searchSession) request(ctx context.Context, method, path string, body io.Reader, contentType string) (data []byte, httpResponse *http.Response, err error) {
	request, err := http.NewRequestWithContext(ctx, method, origin+path, body)
	if err != nil {
		return nil, nil, failure(sdk.InvalidArgument, "could not create ascii2d request", err)
	}
	client := *s.httpClient
	if method == http.MethodPost {
		s.applyFormHeaders(request)
		request.Header.Set(headerContentType, contentType)
		// Upload needs the original Location to recover the query hash.
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	} else {
		site, referer := fetchSameOrigin, homeURL
		if path == homePath {
			site, referer = "none", ""
		}
		s.applyNavigationHeaders(request, site, referer)
	}
	response, err := client.Do(request)
	if err != nil {
		var requestError *url.Error
		for errors.As(err, &requestError) {
			err = requestError.Err
		}
		reason := sdk.UpstreamUnavailable
		if errors.Is(err, errCrossOrigin) {
			reason = sdk.MalformedUpstreamResponse
		}
		return nil, nil, failure(reason, "ascii2d request failed", err)
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			// A failed close must survive challenge recovery rather than being replayed away.
			err = errors.Join(failure(sdk.UpstreamUnavailable, "could not close ascii2d response", closeErr), err)
		}
		var classified *sdk.Error
		if errors.As(err, &classified) {
			classified.HTTPStatus = response.StatusCode
		}
	}()
	data, err = io.ReadAll(response.Body)
	if ctx.Err() != nil {
		return nil, nil, failure(sdk.UpstreamUnavailable, "ascii2d request canceled", ctx.Err())
	}
	if err != nil {
		return nil, nil, failure(sdk.UpstreamUnavailable, "could not read ascii2d response", err)
	}
	header := response.Header
	challenged, err := detectChallenge(response.StatusCode, header, data)
	if err != nil {
		return nil, response, failure(sdk.MalformedUpstreamResponse, "could not inspect ascii2d response", err)
	}
	valid := response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices
	if method == http.MethodPost {
		valid = response.StatusCode >= http.StatusMultipleChoices && response.StatusCode < http.StatusBadRequest
	}
	if challenged || !valid {
		reason := sdk.UpstreamError
		switch {
		case challenged:
			reason = sdk.ChallengeRequired
		case response.StatusCode == http.StatusTooManyRequests:
			reason = sdk.RateLimited
		case response.StatusCode == http.StatusForbidden:
			reason = sdk.Forbidden
		case response.StatusCode >= http.StatusInternalServerError:
			reason = sdk.UpstreamUnavailable
		}
		failure := failure(reason, "ascii2d returned an unsuccessful response", nil)
		failure.HTTPStatus = response.StatusCode
		return nil, response, failure
	}
	return data, response, nil
}

func (s *searchSession) upload(ctx context.Context, image []byte, mediaType string) (string, string, error) {
	data, response, err := s.request(ctx, http.MethodGet, homePath, nil, "")
	if err != nil {
		return "", homeURL, err
	}
	token, err := parseUploadForm(bytes.NewReader(data))
	if err != nil {
		failure := failure(sdk.MalformedUpstreamResponse, "ascii2d returned a malformed upload form", err)
		failure.HTTPStatus = response.StatusCode
		return "", homeURL, failure
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField(paramToken, token); err != nil {
		return "", homeURL, failure(sdk.InvalidArgument, "could not encode ascii2d form", err)
	}
	extension := map[string]string{mediaJPEG: ".jpg", mediaPNG: ".png", mediaWEBP: ".webp"}[mediaType]
	part, err := writer.CreatePart(textproto.MIMEHeader{
		headerDisposition: {`form-data; name="` + paramFile + `"; filename="image` + extension + `"`},
		headerContentType: {mediaType},
	})
	if err != nil {
		return "", homeURL, failure(sdk.InvalidArgument, "could not encode ascii2d image", err)
	}
	if _, err := part.Write(image); err != nil {
		return "", homeURL, failure(sdk.InvalidArgument, "could not encode ascii2d image", err)
	}
	if err := writer.Close(); err != nil {
		return "", homeURL, failure(sdk.InvalidArgument, "could not finish ascii2d upload", err)
	}
	_, response, err = s.request(ctx, http.MethodPost, uploadPath, &body, writer.FormDataContentType())
	if err != nil {
		return "", origin + uploadPath, err
	}
	hash, err := validateUploadLocation(response.Header.Get(headerLocation))
	var classified *sdk.Error
	if errors.As(err, &classified) {
		classified.HTTPStatus = response.StatusCode
	}
	return hash, origin + uploadPath, err
}

func (s *searchSession) search(ctx context.Context, mode Mode, hash string) ([]Match, error) {
	data, response, err := s.request(ctx, http.MethodGet, searchPath+string(mode)+"/"+hash, nil, "")
	if err != nil {
		return nil, err
	}
	matches, err := parseResults(bytes.NewReader(data))
	if err != nil {
		failure := failure(sdk.MalformedUpstreamResponse, "ascii2d returned a malformed result page", err)
		failure.HTTPStatus = response.StatusCode
		return nil, failure
	}
	return matches, nil
}

func validateUploadLocation(raw string) (string, error) {
	malformed := func() (string, error) {
		return "", failure(sdk.MalformedUpstreamResponse, "ascii2d returned a malformed upload location", nil)
	}
	location, err := url.Parse(raw)
	if err != nil || raw == "" || location.User != nil {
		return malformed()
	}
	endpoint, _ := url.Parse(origin)
	// The service emits same-host HTTP Locations for HTTPS uploads; normalize before use.
	if strings.EqualFold(location.Scheme, "http") && strings.EqualFold(location.Hostname(), endpoint.Hostname()) {
		location.Scheme = endpoint.Scheme
	}
	resolved := endpoint.ResolveReference(location)
	if !sameOrigin(endpoint, resolved) || resolved.RawQuery != "" || resolved.ForceQuery || resolved.Fragment != "" {
		return malformed()
	}
	segments := strings.Split(strings.TrimPrefix(resolved.EscapedPath(), "/"), "/")
	if len(segments) != 3 || segments[0] != "search" || segments[1] != string(Color) || !uploadHashPattern.MatchString(segments[2]) {
		return malformed()
	}
	return segments[2], nil
}

func sameOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Hostname(), right.Hostname()) && effectivePort(left) == effectivePort(right)
}

func effectivePort(value *url.URL) string {
	if value.Port() != "" {
		return value.Port()
	}
	if strings.EqualFold(value.Scheme, "https") {
		return defaultHTTPSPort
	}
	return defaultHTTPPort
}
