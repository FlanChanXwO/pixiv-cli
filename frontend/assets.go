// Package frontend 嵌入 MCP gallery 与登录页的 HTML、CSS、JS 源码，并在加载时内联
// 样式和脚本。
//
// HTTP 与 MCP 的行为属于各自的 owner，本包只提供渲染好的页面内容。
// [Pages] 的组合由 [MustLoad] 一次性建立；嵌入路径写死，避免把 fixtures 或开发文件
// 打进可执行文件。
package frontend

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

// 逐文件列出嵌入路径，避免把 fixtures 与开发文件打进可执行文件。
//
//go:embed mcp-apps/gallery/index.html mcp-apps/gallery/gallery.css mcp-apps/gallery/gallery.js
//go:embed pages/oauth/consent.html pages/oauth/consent.css
//go:embed pages/login/layout.html pages/login/manual.html pages/login/callback-relay.html pages/login/result.html pages/login/page.css
var sources embed.FS

// Pages 保存已内联样式的自包含 HTML 与只读登录模板/样式。
type Pages struct {
	GalleryHTML string
	ConsentHTML string
	Login       fs.FS
}

var pages, loadErr = load(sources)

// MustLoad 在 owner 初始化时暴露损坏的嵌入源码，行为类似 template.Must。
func MustLoad() Pages {
	if loadErr != nil {
		panic(loadErr)
	}
	return pages
}

type insertion struct{ marker, source, tag string }

func load(source fs.FS) (Pages, error) {
	gallery, err := inline(source, "mcp-apps/gallery/index.html", []insertion{
		{"<!-- frontend:style -->", "mcp-apps/gallery/gallery.css", "style"},
		{"<!-- frontend:script -->", "mcp-apps/gallery/gallery.js", "script"},
	})
	if err != nil {
		return Pages{}, err
	}
	consent, err := inline(source, "pages/oauth/consent.html", []insertion{
		{"<!-- frontend:style -->", "pages/oauth/consent.css", "style"},
	})
	if err != nil {
		return Pages{}, err
	}
	for _, name := range []string{"layout.html", "manual.html", "callback-relay.html", "result.html", "page.css"} {
		if _, err := fs.ReadFile(source, "pages/login/"+name); err != nil {
			return Pages{}, fmt.Errorf("read login %s: %w", name, err)
		}
	}
	login, err := fs.Sub(source, "pages/login")
	if err != nil {
		return Pages{}, err
	}
	return Pages{GalleryHTML: gallery, ConsentHTML: consent, Login: login}, nil
}

func inline(source fs.FS, name string, parts []insertion) (string, error) {
	content, err := fs.ReadFile(source, name)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", name, err)
	}
	page := string(content)
	for _, part := range parts {
		if count := strings.Count(page, part.marker); count != 1 {
			return "", fmt.Errorf("%s: expected one %s placeholder, found %d", name, part.marker, count)
		}
		value, err := fs.ReadFile(source, part.source)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", part.source, err)
		}
		page = strings.Replace(page, part.marker, "<"+part.tag+">"+string(value)+"</"+part.tag+">", 1)
	}
	return page, nil
}
