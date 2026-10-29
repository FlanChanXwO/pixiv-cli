package api_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/fanbox/api"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/stretchr/testify/require"
)

func TestRequestLogsKeepSecretsPrivate(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(fmt.Sprint(failed), func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Cookie") != "FANBOXSESSID=session-canary" || r.URL.Query().Get(api.ParamPostID) != "query-canary" {
					t.Error("request did not carry the Cookie and query canaries")
				}
				fmt.Fprint(w, `{"payload":"body-canary"}`)
			}))
			defer server.Close()
			httpClient := server.Client()
			transport := httpClient.Transport.(*http.Transport)
			transport.TLSClientConfig.ServerName = server.Listener.Addr().(*net.TCPAddr).IP.String()
			transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
			}
			if failed {
				server.Close()
			}
			var output bytes.Buffer
			logger, err := logging.New(&output, "debug", logging.FormatJSON)
			require.NoError(t, err)
			session, err := api.New("session-canary", api.Options{HTTPClient: httpClient, Logger: logger})
			require.NoError(t, err)
			var response map[string]any
			endpoint := api.APIBaseURL + api.PathPostInfo + "?postId=query-canary&signature=signature-canary"
			err = session.GetJSONURL(t.Context(), endpoint, &response)
			require.Equal(t, failed, err != nil)
			require.Contains(t, output.String(), api.PathPostInfo)
			for _, secret := range []string{"session-canary", "query-canary", "signature-canary", "body-canary", endpoint} {
				require.NotContains(t, output.String(), secret)
			}
		})
	}
}
