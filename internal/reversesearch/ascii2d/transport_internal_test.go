package ascii2d

import (
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
