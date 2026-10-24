package loginhelper

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlerManifestPersistsOnlyAssociationMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	manifest := handlerManifest{Version: 1, ExecutablePath: "/opt/pixiv/bin/pixiv", PreviousHandler: "com.example.pixiv"}
	require.NoError(t, saveHandlerManifest(manifest))

	path, err := handlerManifestPath()
	require.NoError(t, err)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "relay_secret")
	assert.NotContains(t, string(body), "secret")

	loaded, exists, err := loadHandlerManifest()
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, manifest, loaded)
	require.NoError(t, removeHandlerManifest())
	_, exists, err = loadHandlerManifest()
	require.NoError(t, err)
	assert.False(t, exists)
}
