package ascii2d_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	reversesearch "github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch/ascii2d"
	"github.com/stretchr/testify/require"
)

func TestUploadUsesBrowserHeadersAndStableOrder(t *testing.T) {
	headers := uploadWithCapturedHeaders(t, "")
	require.Len(t, headers, 2)

	navigation := headers[0]
	require.Contains(t, navigation.Get("User-Agent"), "Chrome/146.")
	require.Contains(t, navigation.Get("Sec-CH-UA"), `"Chromium";v="146"`)
	require.Contains(t, navigation.Get("Sec-CH-UA"), `"Google Chrome";v="146"`)
	require.Equal(t, "?0", navigation.Get("Sec-CH-UA-Mobile"))
	require.Equal(t, `"macOS"`, navigation.Get("Sec-CH-UA-Platform"))

	form := headers[1]
	require.Equal(t, "https://ascii2d.invalid", form.Get("Origin"))
	require.Equal(t, "https://ascii2d.invalid/", form.Get("Referer"))
	require.Equal(t, "same-origin", form.Get("Sec-Fetch-Site"))
	require.Equal(t, "navigate", form.Get("Sec-Fetch-Mode"))
	require.Equal(t, "?1", form.Get("Sec-Fetch-User"))
	require.Equal(t, "document", form.Get("Sec-Fetch-Dest"))
}

func TestUploadDoesNotSendClientHintsForNonChromiumUserAgent(t *testing.T) {
	const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7; rv:148.0) Gecko/20100101 Firefox/148.0"
	headers := uploadWithCapturedHeaders(t, userAgent)
	require.Len(t, headers, 2)
	for _, header := range headers {
		require.Equal(t, userAgent, header.Get("User-Agent"))
		for _, key := range []string{"Sec-CH-UA", "Sec-CH-UA-Mobile", "Sec-CH-UA-Platform"} {
			require.Empty(t, header.Values(key), "non-Chromium user-agent must not send %s", key)
		}
	}
}

func TestUploadUsesChromiumVersionFromCustomUserAgent(t *testing.T) {
	const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"
	headers := uploadWithCapturedHeaders(t, userAgent)
	require.Len(t, headers, 2)
	require.Equal(t, userAgent, headers[0].Get("User-Agent"))
	require.Contains(t, headers[0].Get("Sec-CH-UA"), `"Chromium";v="145"`)
	require.Contains(t, headers[0].Get("Sec-CH-UA"), `"Google Chrome";v="145"`)
	require.Equal(t, `"Windows"`, headers[0].Get("Sec-CH-UA-Platform"))
	require.Equal(t, "?0", headers[0].Get("Sec-CH-UA-Mobile"))
}

func TestNewRejectsUserAgentHeaderInjection(t *testing.T) {
	for _, userAgent := range []string{
		"Mozilla/5.0\r\nX-Injected: yes",
		"Mozilla/5.0\nX-Injected: yes",
		"Mozilla/5.0\x00X-Injected: yes",
	} {
		t.Run(strings.ReplaceAll(strings.ReplaceAll(userAgent, "\r", "CR"), "\n", "LF"), func(t *testing.T) {
			_, err := ascii2d.New(ascii2d.Options{UserAgent: userAgent})
			require.Equal(t, reversesearch.CodeInvalidRequest, reversesearch.CodeOf(err))
			require.EqualError(t, err, "ascii2d user-agent contains invalid header characters")
		})
	}
}

func uploadWithCapturedHeaders(t *testing.T, userAgent string) []http.Header {
	t.Helper()
	requests := make([]http.Header, 0, 2)
	client, err := ascii2d.New(ascii2d.Options{
		Endpoint:  "https://ascii2d.invalid",
		UserAgent: userAgent,
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests = append(requests, request.Header.Clone())
			switch request.Method {
			case http.MethodGet:
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(uploadFormFixture())),
					Request:    request,
				}, nil
			case http.MethodPost:
				_, _ = io.Copy(io.Discard, request.Body)
				_ = request.Body.Close()
				return &http.Response{
					StatusCode: http.StatusFound,
					Header:     http.Header{"Location": {"/search/color/" + fixtureHash}},
					Body:       io.NopCloser(strings.NewReader("")),
					Request:    request,
				}, nil
			default:
				return nil, errors.New("unexpected ascii2d test request method")
			}
		})},
	})
	require.NoError(t, err)

	_, err = client.Upload(context.Background(), loadSnapshot(t, []byte("\x89PNG\r\n\x1a\nfixture")))
	require.NoError(t, err)
	return requests
}
