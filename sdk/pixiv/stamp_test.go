package pixiv_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

func TestStampEndpointContracts(t *testing.T) {
	testEndpointContracts(t, []endpointContract{
		{"stamps", api.PathStamps, `{"stamps":[{"stamp_id":1,"stamp_url":"https://s.pximg.net/stamp.png"}]}`, func(t *testing.T, ctx context.Context, c *pixiv.Client) error {
			stamps, err := c.Stamps(ctx)
			if err == nil {
				require.Len(t, stamps, 1)
				require.EqualValues(t, 1, stamps[0].ID)
				require.Equal(t, "https://s.pximg.net/stamp.png", stamps[0].Image.URL)
			}
			return err
		}},
	})
}

func TestStampMalformedResponses(t *testing.T) {
	for _, body := range []string{
		`{"stamps":null}`, `{"stamps":[{"stamp_id":0,"stamp_url":"https://s.pximg.net/stamp.png"}]}`,
		`{"stamps":[{"stamp_id":1,"stamp_url":"https://evil.example/stamp.png"}]}`,
		`{"stamps":[{"stamp_id":1,"stamp_url":"http://s.pximg.net/stamp.png"}]}`,
		`{"stamps":[],"next_url":"https://app-api.pixiv.net/v1/stamps?offset=1"}`, `{"stamps":[],"next_url":false}`,
	} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			value := pixiv.NewCredentials("fixture-refresh")
			value.SetTokens("fixture-access", "fixture-refresh")
			value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := pixiv.NewMemoryCredentials(value)
			require.NoError(t, err)
			client, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server), ResourceHosts: []string{"evil.example"}})
			require.NoError(t, err)
			_, err = client.Stamps(t.Context())
			require.Equal(t, sdk.MalformedUpstreamResponse, sdk.ReasonOf(err))
		})
	}
}

func TestStampDecodeClearsBinding(t *testing.T) {
	content, err := (&pixiv.Client{}).Novel(23).ParseContent([]byte(novelHTML))
	require.NoError(t, err)
	stamp := pixiv.Stamp{ID: 1, Image: content.Blocks[2].Image.Image}
	require.NoError(t, json.Unmarshal([]byte(`{"id":2}`), &stamp))
	_, err = stamp.Image.Open(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
}
