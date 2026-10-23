package ascii2d

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewUsesChrome146BrowserTransportByDefault(t *testing.T) {
	client, err := New(Options{Endpoint: "https://ascii2d.invalid"})
	require.NoError(t, err)
	require.NotNil(t, client)
	_, ok := client.httpClient.Transport.(*browserTransport)
	require.True(t, ok, "default ascii2d transport must use tls-client browser transport")
}

func TestBrowserHeaderOrderForNavigationAndFormRequests(t *testing.T) {
	navigation := make(http.Header)
	navigation.Set("Sec-CH-UA", `"Not(A:Brand";v="24", "Chromium";v="146", "Google Chrome";v="146"`)
	navigation.Set("Sec-CH-UA-Mobile", "?0")
	navigation.Set("Sec-CH-UA-Platform", `"macOS"`)
	navigation.Set("Upgrade-Insecure-Requests", "1")
	navigation.Set("User-Agent", defaultUserAgent)
	navigation.Set("Accept", browserAccept)
	navigation.Set("Sec-Fetch-Site", "none")
	navigation.Set("Sec-Fetch-Mode", "navigate")
	navigation.Set("Sec-Fetch-User", "?1")
	navigation.Set("Sec-Fetch-Dest", "document")
	navigation.Set("Accept-Encoding", browserEncoding)
	navigation.Set("Accept-Language", browserLanguage)
	require.Equal(t, []string{
		"sec-ch-ua",
		"sec-ch-ua-mobile",
		"sec-ch-ua-platform",
		"upgrade-insecure-requests",
		"user-agent",
		"cookie",
		"accept",
		"sec-fetch-site",
		"sec-fetch-mode",
		"sec-fetch-user",
		"sec-fetch-dest",
		"accept-encoding",
		"accept-language",
	}, browserHeaderOrder(navigation))

	form := navigation.Clone()
	form.Del("Upgrade-Insecure-Requests")
	form.Set("Origin", "https://ascii2d.invalid")
	form.Set("Referer", "https://ascii2d.invalid/")
	form.Set("Content-Type", "multipart/form-data; boundary=fixture")
	require.Equal(t, []string{
		"sec-ch-ua",
		"sec-ch-ua-mobile",
		"sec-ch-ua-platform",
		"user-agent",
		"cookie",
		"accept",
		"origin",
		"referer",
		"sec-fetch-site",
		"sec-fetch-mode",
		"sec-fetch-user",
		"sec-fetch-dest",
		"accept-encoding",
		"accept-language",
		"content-type",
	}, browserHeaderOrder(form))

	firefox := form.Clone()
	firefox.Del("Sec-CH-UA")
	firefox.Del("Sec-CH-UA-Mobile")
	firefox.Del("Sec-CH-UA-Platform")
	require.Equal(t, []string{
		"user-agent",
		"cookie",
		"accept",
		"origin",
		"referer",
		"sec-fetch-site",
		"sec-fetch-mode",
		"sec-fetch-user",
		"sec-fetch-dest",
		"accept-encoding",
		"accept-language",
		"content-type",
	}, browserHeaderOrder(firefox))
}
