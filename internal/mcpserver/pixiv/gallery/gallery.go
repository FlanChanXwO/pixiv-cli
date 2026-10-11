// Package gallery owns the optional Pixiv MCP Apps view, never product credentials.
package gallery

import (
	"context"
	_ "embed"
	htmlutil "html"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const URI = "ui://pixiv-cli/gallery"
const MIME = "text/html;profile=mcp-app"

//go:embed generated/gallery.html
var html string

// Register always publishes the view; non-UI hosts retain the same tool results.
func Register(server *mcp.Server, media ...*mediaproxy.Service) {
	server.AddResource(&mcp.Resource{URI: URI, Name: "pixiv_gallery", MIMEType: MIME, Description: "Pixiv artwork previews through the standard MCP Apps bridge."}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		domains := []string{"blob:"}
		page := html
		if len(media) > 0 && media[0] != nil {
			origin := media[0].Origin()
			domains = append(domains, origin)
			page = strings.Replace(page, "img-src blob:", "img-src blob: "+htmlutil.EscapeString(origin), 1)
		}
		content := &mcp.ResourceContents{URI: URI, MIMEType: MIME, Text: page, Meta: mcp.Meta{
			"ui": map[string]any{"csp": map[string]any{"connectDomains": []string{}, "resourceDomains": domains}, "prefersBorder": true},
		}}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{content}}, nil
	})
}

// Attach associates visual discovery with the resource without replacing its structured contract.
func Attach(tool *mcp.Tool) {
	switch tool.Name {
	case "pixiv_show_gallery":
		if tool.Meta == nil {
			tool.Meta = mcp.Meta{}
		}
		tool.Meta["ui"] = map[string]any{"resourceUri": URI}
	}
}
