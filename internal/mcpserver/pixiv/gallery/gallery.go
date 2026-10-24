// Package gallery owns the optional Pixiv MCP Apps view, never product credentials.
package gallery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	htmlutil "html"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/frontend"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const uri = "ui://pixiv-cli/gallery"
const mime = "text/html;profile=mcp-app"

var html = frontend.MustLoad().GalleryHTML

func versionURI(media *mediaproxy.Service) string {
	page, _ := pageAssets(media)
	digest := sha256.Sum256([]byte(page))
	return uri + "/sha256-" + hex.EncodeToString(digest[:])
}

// Register always publishes the view; non-UI hosts retain the same tool results.
func Register(server *mcp.Server, media *mediaproxy.Service) {
	read := func(_ context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		requestURI := request.Params.URI
		if requestURI != uri {
			digest := strings.TrimPrefix(requestURI, uri+"/sha256-")
			if len(digest) != hex.EncodedLen(sha256.Size) || digest != strings.ToLower(digest) {
				return nil, errors.New("Gallery version unavailable")
			}
			if _, err := hex.DecodeString(digest); err != nil {
				return nil, errors.New("Gallery version unavailable")
			}
		}
		page, domains := pageAssets(media)
		content := &mcp.ResourceContents{URI: requestURI, MIMEType: mime, Text: page, Meta: mcp.Meta{
			"ui": map[string]any{"csp": map[string]any{"connectDomains": []string{}, "resourceDomains": domains}, "permissions": map[string]any{"clipboardWrite": map[string]any{}}, "prefersBorder": false},
		}}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{content}}, nil
	}
	server.AddResource(&mcp.Resource{URI: uri, Name: "pixiv_gallery", MIMEType: mime, Description: "Pixiv artwork previews through the standard MCP Apps bridge."}, read)
	server.AddResource(&mcp.Resource{URI: versionURI(media), Name: "pixiv_gallery_versioned", MIMEType: mime, Description: "Current Gallery UI revision for host resource caching."}, read)
	// Keep historical view references readable after a build; revisions are cache keys, not integrity proofs.
	server.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: uri + "/sha256-{digest}", Name: "pixiv_gallery_history", MIMEType: mime}, read)
}

// Attach associates discovery and complete media with the view without replacing structured results.
func Attach(tool *mcp.Tool, media *mediaproxy.Service) {
	switch tool.Name {
	case "pixiv_show_gallery", "pixiv_show_artwork", "pixiv_artwork_media":
		if tool.Meta == nil {
			tool.Meta = mcp.Meta{}
		}
		tool.Meta["ui"] = map[string]any{"resourceUri": versionURI(media)}
	}
}

func pageAssets(media *mediaproxy.Service) (string, []string) {
	page := html
	domains := []string{"blob:"}
	if media != nil {
		origin := media.Origin()
		domains = append(domains, origin)
		page = strings.Replace(page, "img-src blob:", "img-src blob: "+htmlutil.EscapeString(origin), 1)
	}
	return page, domains
}
