package ascii2d_test

import (
	"context"
	"errors"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch/ascii2d"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
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

func TestUploadBrowserHeadersMatchUserAgent(t *testing.T) {
	for _, test := range []struct {
		name      string
		userAgent string
		wantHints bool
		platform  string
		version   string
	}{
		{
			name:      "firefox omits client hints",
			userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7; rv:148.0) Gecko/20100101 Firefox/148.0",
			platform:  "",
		},
		{
			name:      "chromium version from custom user agent",
			userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
			wantHints: true,
			platform:  "Windows",
			version:   "145",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			headers := uploadWithCapturedHeaders(t, test.userAgent)
			require.Len(t, headers, 2)
			for _, header := range headers {
				require.Equal(t, test.userAgent, header.Get("User-Agent"))
				if !test.wantHints {
					for _, key := range []string{"Sec-CH-UA", "Sec-CH-UA-Mobile", "Sec-CH-UA-Platform"} {
						require.Empty(t, header.Values(key), "non-Chromium user-agent must not send %s", key)
					}
					continue
				}
				require.Contains(t, header.Get("Sec-CH-UA"), `"Chromium";v="`+test.version+`"`)
				require.Contains(t, header.Get("Sec-CH-UA"), `"Google Chrome";v="`+test.version+`"`)
				require.Equal(t, `"`+test.platform+`"`, header.Get("Sec-CH-UA-Platform"))
				require.Equal(t, "?0", header.Get("Sec-CH-UA-Mobile"))
			}
		})
	}
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
