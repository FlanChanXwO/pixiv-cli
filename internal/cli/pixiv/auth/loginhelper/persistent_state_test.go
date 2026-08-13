package loginhelper_test

import (
	"os"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/auth/loginhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlerManifestPersistsOnlyAssociationMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	manifest := loginhelper.HandlerManifest{Version: 1, ExecutablePath: "/opt/pixiv/bin/pixiv", PreviousHandler: "com.example.pixiv"}
	require.NoError(t, loginhelper.SaveHandlerManifest(manifest))

	path, err := loginhelper.HandlerManifestPath()
	require.NoError(t, err)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "relay_secret")
	assert.NotContains(t, string(body), "secret")

	loaded, exists, err := loginhelper.LoadHandlerManifest()
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, manifest, loaded)
	require.NoError(t, loginhelper.RemoveHandlerManifest())
	_, exists, err = loginhelper.LoadHandlerManifest()
	require.NoError(t, err)
	assert.False(t, exists)
}
