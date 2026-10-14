package artwork_media

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAppAnimationUsesPublishedArtifactWithoutChangingLegacyContent(t *testing.T) {
	original := &mcp.ImageContent{MIMEType: "image/gif", Data: []byte("complete-original")}
	preview := &mcp.ImageContent{MIMEType: "image/png", Data: []byte("preview")}
	contents := []mcp.Content{original, preview}
	page := outputs.MediaPage{Page: 1, Filename: "42.gif", MIMEType: "image/gif", Size: len(original.Data), ContentIndex: 0, PreviewContentIndex: new(1)}
	request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Meta: mcp.Meta{"io.modelcontextprotocol/clientCapabilities": map[string]any{"extensions": map[string]any{"io.modelcontextprotocol/ui": map[string]any{"mimeTypes": []string{"text/html;profile=mcp-app"}}}}}}}
	url := "https://media.test/media/animation/opaque.animation.signature"
	downloads := map[string]any{"media_origin": "https://media.test", "pages": []map[string]any{{"page": 1, "url": url}}}
	result := &mcp.CallToolResult{Content: contents, Meta: mcp.Meta{"pixiv/mediaDownloads": downloads}}
	appAnimationContent(request, result, outputs.ArtworkMedia{Complete: true, Pages: []outputs.MediaPage{page}})
	require.Equal(t, []mcp.Content{preview}, result.Content)
	appContents, ok := result.Meta["pixiv/mediaContents"].([]mcp.Content)
	require.True(t, ok)
	link, ok := appContents[0].(*mcp.ResourceLink)
	require.True(t, ok, "published App media must use protected HTTP instead of another large control payload")
	require.Equal(t, url, link.URI)
	require.Equal(t, "42.gif", link.Name)
	require.Equal(t, "image/gif", link.MIMEType)
	require.NotNil(t, link.Size)
	require.EqualValues(t, len(original.Data), *link.Size)
	require.Same(t, original, contents[0], "negotiated delivery must not mutate the legacy content slice")
	require.Same(t, preview, appContents[1])
	require.Equal(t, downloads, result.Meta["pixiv/mediaDownloads"])
}
