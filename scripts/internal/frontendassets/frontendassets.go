// Package frontendassets prepares the embedded pages from their frontend sources.
package frontendassets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type asset struct {
	source, output string
	inline         []insertion
}
type insertion struct{ marker, source, tag string }

var assets = []asset{
	{"frontend/mcp-apps/gallery/index.html", "internal/mcpserver/pixiv/gallery/generated/gallery.html", []insertion{
		{"<!-- frontend:style -->", "frontend/mcp-apps/gallery/gallery.css", "style"},
		{"<!-- frontend:script -->", "frontend/mcp-apps/gallery/gallery.js", "script"},
	}},
	{"frontend/pages/oauth/consent.html", "internal/mcpserver/auth/generated/consent.html", []insertion{
		{"<!-- frontend:style -->", "frontend/pages/oauth/consent.css", "style"},
	}},
	{"frontend/pages/login/layout.html", "internal/services/pixiv/account/loginrelay/loginpage/generated/templates/layout.html", nil},
	{"frontend/pages/login/manual.html", "internal/services/pixiv/account/loginrelay/loginpage/generated/templates/manual.html", nil},
	{"frontend/pages/login/callback-relay.html", "internal/services/pixiv/account/loginrelay/loginpage/generated/templates/callback-relay.html", nil},
	{"frontend/pages/login/result.html", "internal/services/pixiv/account/loginrelay/loginpage/generated/templates/result.html", nil},
	{"frontend/pages/login/page.css", "internal/services/pixiv/account/loginrelay/loginpage/generated/page.css", nil},
}

// Generate writes deterministic assets, or checks them without writing when check is true.
func Generate(root string, check bool) error {
	outputs := make([][]byte, len(assets))
	// Validate every input before writing so broken sources never leave a partial generation.
	for i, a := range assets {
		content, err := os.ReadFile(filepath.Join(root, a.source))
		if err != nil {
			return fmt.Errorf("read %s: %w", a.source, err)
		}
		page := string(content)
		for _, part := range a.inline {
			if count := strings.Count(page, part.marker); count != 1 {
				return fmt.Errorf("%s: expected one %s placeholder, found %d", a.source, part.marker, count)
			}
			value, err := os.ReadFile(filepath.Join(root, part.source))
			if err != nil {
				return fmt.Errorf("read %s: %w", part.source, err)
			}
			page = strings.ReplaceAll(page, part.marker, "<"+part.tag+">"+string(value)+"</"+part.tag+">")
		}
		outputs[i] = []byte(page)
	}
	for i, a := range assets {
		path := filepath.Join(root, a.output)
		if check {
			current, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("check %s: %w; run go run ./scripts/cmd/frontendassets", a.output, err)
			}
			if string(current) != string(outputs[i]) {
				return fmt.Errorf("%s is stale; run go run ./scripts/cmd/frontendassets", a.output)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Errorf("create %s: %w", a.output, err)
		}
		if err := os.WriteFile(path, outputs[i], 0644); err != nil {
			return fmt.Errorf("write %s: %w", a.output, err)
		}
	}
	return nil
}
