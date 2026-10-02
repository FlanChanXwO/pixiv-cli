// Package gallery owns the optional Pixiv MCP Apps view, never product credentials.
package gallery

import (
	"context"
	_ "embed"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const URI = "ui://pixiv-cli/gallery"
const MIME = "text/html;profile=mcp-app"

//go:embed gallery.html
var html string

// Register always publishes the view; non-UI hosts retain the same tool results.
func Register(server *mcp.Server) {
	server.AddResource(&mcp.Resource{URI: URI, Name: "pixiv_gallery", MIMEType: MIME, Description: "Pixiv artwork previews through the standard MCP Apps bridge."}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		content := &mcp.ResourceContents{URI: URI, MIMEType: MIME, Text: html, Meta: mcp.Meta{
			"ui": map[string]any{"csp": map[string]any{"connectDomains": []string{}, "resourceDomains": []string{"blob:"}}, "prefersBorder": true},
		}}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{content}}, nil
	})
}

// Attach associates visual discovery with the resource without replacing its structured contract.
func Attach(tool *mcp.Tool) {
	switch tool.Name {
	case "pixiv_search_illust", "pixiv_illust_detail", "pixiv_illust_related", "pixiv_illust_ranking", "pixiv_illust_recommended", "pixiv_recommended":
		if tool.Meta == nil {
			tool.Meta = mcp.Meta{}
		}
		tool.Meta["ui"] = map[string]any{"resourceUri": URI}
	}
}
