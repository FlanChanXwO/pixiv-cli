// Package frontend embeds the page sources; HTTP and MCP behavior remains in their owners.
package frontend

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

// Exact paths keep fixtures and developer files out of the executable.
//
//go:embed mcp-apps/gallery/index.html mcp-apps/gallery/gallery.css mcp-apps/gallery/gallery.js
//go:embed pages/oauth/consent.html pages/oauth/consent.css
//go:embed pages/login/layout.html pages/login/manual.html pages/login/callback-relay.html pages/login/result.html pages/login/page.css
var sources embed.FS

// Pages contains self-contained HTML and the read-only login templates/styles.
type Pages struct {
	GalleryHTML string
	ConsentHTML string
	Login       fs.FS
}

var pages, loadErr = load(sources)

// MustLoad exposes broken embedded sources during owner initialization, like template.Must.
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
