package pixivapi_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	"github.com/stretchr/testify/require"
)

func TestRequestLogsKeepSecretsPrivate(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(fmt.Sprint(failed), func(t *testing.T) {
			api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer access-canary" || r.URL.Query().Get("word") != "query-canary" {
					t.Error("request did not carry the credential and query canaries")
				}
				fmt.Fprint(w, `{"payload":"body-canary"}`)
			}))
			defer api.Close()
			httpClient := api.Client()
			transport := httpClient.Transport.(*http.Transport)
			transport.TLSClientConfig.ServerName = api.Listener.Addr().(*net.TCPAddr).IP.String()
			transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, api.Listener.Addr().String())
			}
			if failed {
				api.Close()
			}
			var output bytes.Buffer
			logger, err := logging.New(&output, "debug", logging.FormatJSON)
			require.NoError(t, err)
			client := pixivapi.New(pixivapi.Config{HTTPClient: httpClient, AccessToken: "access-canary", Logger: logger})
			query := url.Values{pixivapi.ParamWord: {"query-canary"}, "url": {"https://i.pximg.net/image.png?signature=signature-canary"}}
			var response map[string]any
			err = client.GetJSON(t.Context(), pixivapi.PathSearchIllust, query, &response)
			require.Equal(t, failed, err != nil)
			require.Contains(t, output.String(), pixivapi.PathSearchIllust)
			for _, secret := range []string{"access-canary", "query-canary", "signature-canary", "body-canary", "https://i.pximg.net"} {
				require.NotContains(t, output.String(), secret)
			}
		})
	}
}
