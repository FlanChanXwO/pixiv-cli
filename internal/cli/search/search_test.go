package search_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/search"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

func TestSearchOutputContract(t *testing.T) {
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"illusts":[{"id":101,"title":"cat","type":"illust","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7,"name":"artist"},"tags":[]}],"next_url":null}`)
	}))
	defer api.Close()
	httpClient := api.Client()
	transport := httpClient.Transport.(*http.Transport)
	transport.TLSClientConfig.ServerName = api.Listener.Addr().(*net.TCPAddr).IP.String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, api.Listener.Addr().String())
	}
	client, err := pixiv.NewWith("access-canary", pixiv.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	for _, format := range []string{"text", "json", "ndjson"} {
		t.Run(format, func(t *testing.T) {
			var output, diagnostics bytes.Buffer
			env := command.Env{In: strings.NewReader(""), Out: &output, Err: &diagnostics, ResolveJSON: func(override *bool) (bool, error) { return override != nil && *override, nil }}
			cmd := search.New(env, func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
				_, err := attempt(ctx, client)
				return err
			}, nil)
			args := []string{"cat", "--limit", "1"}
			if format != "text" {
				args = append(args, "--"+format)
			}
			cmd.SetArgs(args)
			require.NoError(t, cmd.Execute())
			want, err := os.ReadFile(filepath.Join("testdata", "search-"+format+".golden"))
			require.NoError(t, err)
			require.Equal(t, string(want), output.String())
			require.Empty(t, diagnostics.String())
		})
	}
}
