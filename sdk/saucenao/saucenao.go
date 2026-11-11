// Package saucenao searches SauceNAO using caller-supplied image bytes and an API key.
//
// [Search] returns ranked [Match] values and [Quota] without reading local
// configuration or account files. Callers own image loading and result merging.
package saucenao

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

const (
	endpoint          = "https://saucenao.com/search.php"
	operationSearch   = "saucenao.Search"
	headerContentType = "Content-Type"
	paramAPIKey       = "api_key"
	paramOutput       = "output_type"
	paramDatabase     = "db"
	paramFile         = "file"
	outputJSON        = "2"
	allDatabases      = "999"
	imageFilename     = "image"
)

var errCrossOrigin = errors.New("SauceNAO redirect is outside its origin")

// Options supplies the image, credentials, and caller-owned HTTP connection.
type Options struct {
	Image      []byte       `json:"-"` // Image must remain unchanged until Search returns.
	APIKey     string       `json:"-"`
	HTTPClient *http.Client `json:"-"` // Nil uses http.DefaultClient.
}

// Result retains provider order and the quota reported by SauceNAO.
// Quota is nil when the upstream did not supply a complete quota summary.
type Result struct {
	Matches []Match `json:"matches"`
	Quota   *Quota  `json:"quota,omitempty"`
}

// Quota reports remaining requests and limits for the upstream's short and long windows.
type Quota struct {
	ShortRemaining int64 `json:"short_remaining"`
	LongRemaining  int64 `json:"long_remaining"`
	ShortLimit     int64 `json:"short_limit"`
	LongLimit      int64 `json:"long_limit"`
}

// Match retains one ranked upstream result. ArtworkID and UserID are zero
// when no corresponding Pixiv identity is available; Similarity is a percentage.
type Match struct {
	Rank         int      `json:"rank"`
	Similarity   float64  `json:"similarity"`
	IndexID      int64    `json:"index_id"`
	IndexName    string   `json:"index_name"`
	Title        string   `json:"title,omitempty"`
	Author       string   `json:"author,omitempty"`
	ArtworkID    int64    `json:"artwork_id,omitempty"`
	UserID       int64    `json:"user_id,omitempty"`
	ExternalURLs []string `json:"external_urls,omitempty"`
}

// Search uploads Image once and returns results with any reported quota.
// Failures use sdk.Error and preserve context cancellation for errors.Is.
func Search(ctx context.Context, options Options) (result Result, err error) {
	if err := ctx.Err(); err != nil {
		return Result{}, failure(sdk.UpstreamUnavailable, "SauceNAO search canceled", err)
	}
	if strings.TrimSpace(options.APIKey) == "" {
		return Result{}, failure(sdk.Unauthorized, "SauceNAO API key is required", nil)
	}
	if len(options.Image) == 0 {
		return Result{}, failure(sdk.InvalidArgument, "image bytes are required", nil)
	}
	// 1. Encode the caller's in-memory image without a background upload worker.
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, field := range []struct{ name, value string }{
		{paramAPIKey, options.APIKey}, {paramOutput, outputJSON}, {paramDatabase, allDatabases},
	} {
		if err := writer.WriteField(field.name, field.value); err != nil {
			return Result{}, failure(sdk.InvalidArgument, "could not encode SauceNAO search", err)
		}
	}
	part, err := writer.CreateFormFile(paramFile, imageFilename)
	if err != nil {
		return Result{}, failure(sdk.InvalidArgument, "could not encode SauceNAO image", err)
	}
	if _, err := part.Write(options.Image); err != nil {
		return Result{}, failure(sdk.InvalidArgument, "could not encode SauceNAO image", err)
	}
	if err := writer.Close(); err != nil {
		return Result{}, failure(sdk.InvalidArgument, "could not finish SauceNAO search", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return Result{}, failure(sdk.InvalidArgument, "could not create SauceNAO request", err)
	}
	request.Header.Set(headerContentType, writer.FormDataContentType())
	// 2. Keep the API key's multipart body on the service origin during redirects.
	client := http.DefaultClient
	if options.HTTPClient != nil {
		client = options.HTTPClient
	}
	connection := *client
	connection.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if next.URL.Scheme != request.URL.Scheme || !strings.EqualFold(next.URL.Host, request.URL.Host) {
			return errCrossOrigin
		}
		if client.CheckRedirect != nil {
			return client.CheckRedirect(next, via)
		}
		if len(via) >= 10 { // Retain net/http's default redirect limit.
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	response, err := connection.Do(request)
	if err != nil {
		reason := sdk.UpstreamUnavailable
		if errors.Is(err, errCrossOrigin) {
			reason = sdk.ResourceForbidden
		}
		return Result{}, failure(reason, "SauceNAO request failed", err)
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			err = errors.Join(err, failure(sdk.UpstreamUnavailable, "could not close SauceNAO response", closeErr))
		}
	}()
	// 3. Classify HTTP failures before decoding the service's JSON envelope.
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		reason := sdk.UpstreamError
		switch response.StatusCode {
		case http.StatusUnauthorized:
			reason = sdk.Unauthorized
		case http.StatusForbidden:
			reason = sdk.Forbidden
		case http.StatusTooManyRequests:
			reason = sdk.RateLimited
		default:
			if response.StatusCode >= http.StatusInternalServerError {
				reason = sdk.UpstreamUnavailable
			}
		}
		failure := failure(reason, "SauceNAO returned an HTTP failure", nil)
		failure.HTTPStatus = response.StatusCode
		return Result{}, failure
	}
	result, err = decodeResponse(response.Body)
	if canceled := ctx.Err(); canceled != nil {
		return result, failure(sdk.UpstreamUnavailable, "SauceNAO search canceled", canceled)
	}
	var classified *sdk.Error
	if errors.As(err, &classified) {
		classified.HTTPStatus = response.StatusCode
	}
	return result, err
}

func failure(reason sdk.Reason, message string, cause error) *sdk.Error {
	// net/http wraps transport errors with URLs that may contain redirected queries.
	var requestError *url.Error
	for errors.As(cause, &requestError) {
		cause = requestError.Err
	}
	return &sdk.Error{Reason: reason, Operation: operationSearch, Message: message, Err: cause}
}
