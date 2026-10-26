// ascii2d image upload: CSRF-protected multipart request and redirect validation.
// Session construction lives in ascii2d.go.

package ascii2d

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"

	reversesearch "github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
)

func (c *Client) upload(ctx context.Context, snapshot *reversesearch.Snapshot, mediaType, token string) (string, error) {
	body, contentType, writeResult := uploadBody(ctx, snapshot, mediaType, token)
	uploadURL := c.resolvePath("/search/file")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL.String(), body)
	if err != nil {
		_ = body.CloseWithError(err)
		<-writeResult
		return "", reversesearch.NewError(reversesearch.CodeProviderFailed, "could not create ascii2d upload request", err)
	}
	c.applyFormHeaders(request)
	request.Header.Set("Content-Type", contentType)

	client := *c.httpClient
	// 上传必须观察原始 Location；自动跟随会丢失服务端生成的查询 hash。
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		_ = body.CloseWithError(err)
		<-writeResult
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", reversesearch.NewError(reversesearch.CodeProviderFailed, "ascii2d upload request failed", err)
	}
	defer response.Body.Close()
	writeErr := <-writeResult
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	if response.StatusCode >= http.StatusMultipleChoices && response.StatusCode < http.StatusBadRequest {
		if challengeErr := detectResponseChallenge(ctx, response); challengeErr != nil {
			return "", challengeErr
		}
	} else {
		return "", classifyResponseError(ctx, response, "ascii2d returned an unsuccessful HTTP status")
	}
	hash, locationErr := c.validateUploadLocation(response.Header.Get("Location"))
	if locationErr != nil {
		return "", locationErr
	}
	if writeErr != nil {
		return "", reversesearch.NewError(reversesearch.CodeProviderFailed, "could not upload image to ascii2d", writeErr)
	}
	return hash, nil
}

func uploadBody(ctx context.Context, snapshot *reversesearch.Snapshot, mediaType, token string) (*io.PipeReader, string, <-chan error) {
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	result := make(chan error, 1)
	go func() {
		err := multipartWriter.WriteField("authenticity_token", token)
		if err == nil {
			filename := "image" + mediaExtension(mediaType)
			header := textproto.MIMEHeader{
				"Content-Disposition": {fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename)},
				"Content-Type":        {mediaType},
			}
			var part io.Writer
			part, err = multipartWriter.CreatePart(header)
			if err == nil {
				var source io.ReadCloser
				source, err = snapshot.Open(ctx)
				if err == nil {
					_, err = io.Copy(part, source)
					err = errors.Join(err, source.Close())
				}
			}
		}
		err = errors.Join(err, multipartWriter.Close())
		if err != nil {
			_ = writer.CloseWithError(err)
		} else {
			err = writer.Close()
		}
		result <- err
		close(result)
	}()
	return reader, multipartWriter.FormDataContentType(), result
}

func mediaExtension(mediaType string) string {
	switch mediaType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}

func (c *Client) validateUploadLocation(raw string) (string, error) {
	location, err := url.Parse(raw)
	if err != nil || raw == "" || location.User != nil {
		return "", reversesearch.NewError(reversesearch.CodeMalformedUpstreamResponse, "ascii2d returned a malformed upload location", nil)
	}
	normalizeSameHostHTTPSUploadLocation(c.endpoint, location)
	resolved := c.endpoint.ResolveReference(location)
	if !sameOrigin(c.endpoint, resolved) {
		return "", reversesearch.NewError(reversesearch.CodeMalformedUpstreamResponse, "ascii2d returned an unsafe upload location", nil)
	}
	if resolved.RawQuery != "" || resolved.Fragment != "" {
		return "", reversesearch.NewError(reversesearch.CodeMalformedUpstreamResponse, "ascii2d returned a malformed upload location", nil)
	}
	segments := strings.Split(strings.TrimPrefix(resolved.EscapedPath(), "/"), "/")
	if len(segments) != 3 || segments[0] != "search" || segments[1] != "color" || !uploadHashPattern.MatchString(segments[2]) {
		return "", reversesearch.NewError(reversesearch.CodeMalformedUpstreamResponse, "ascii2d returned a malformed upload location", nil)
	}
	return segments[2], nil
}

func normalizeSameHostHTTPSUploadLocation(endpoint, location *url.URL) {
	if !strings.EqualFold(endpoint.Scheme, "https") ||
		!strings.EqualFold(location.Scheme, "http") ||
		!strings.EqualFold(location.Hostname(), endpoint.Hostname()) {
		return
	}
	// ascii2d 当前会在 HTTPS 上传响应中生成同主机的 HTTP Location。这里只
	// 把已校验主机的 scheme 规范回 HTTPS endpoint，绝不跟随不安全的降级地址；
	// 显式端口仍由 sameOrigin 继续校验。
	location.Scheme = endpoint.Scheme
}

func sameOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Hostname(), right.Hostname()) && effectivePort(left) == effectivePort(right)
}

func effectivePort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	if strings.EqualFold(value.Scheme, "https") {
		return "443"
	}
	return "80"
}
