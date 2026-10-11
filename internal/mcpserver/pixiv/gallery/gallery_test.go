package gallery_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/gallery"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The browser-independent fixture uses only Node built-ins, with no npm install.
func TestGalleryBridgeMock(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable; run node frontend/mcp-apps/gallery/gallery_test.mjs to verify the app bridge")
	}
	for _, mode := range []string{"supported", "without-download", "without-tools", "bad-protocol"} {
		t.Run(mode, func(t *testing.T) {
			args := []string{"../../../../frontend/mcp-apps/gallery/gallery_test.mjs"}
			if mode != "supported" {
				args = append(args, "--"+mode)
			}
			output, err := exec.CommandContext(t.Context(), node, args...).CombinedOutput()
			if err != nil {
				t.Fatalf("bridge mock failed: %v\n%s", err, output)
			}
		})
	}
}

func TestGalleryEmbeddedAssetsAndMediaCSP(t *testing.T) {
	for _, withMedia := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "media"}[withMedia], func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "gallery-test", Version: "1"}, nil)
			if withMedia {
				store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
				if _, err := store.Init(t.Context(), false); err != nil {
					t.Fatal(err)
				}
				media, err := mediaproxy.New("https://media.example.test/pixiv", store, nil)
				if err != nil {
					t.Fatal(err)
				}
				gallery.Register(server, media)
			} else {
				gallery.Register(server)
			}
			c, s := mcp.NewInMemoryTransports()
			ss, err := server.Connect(t.Context(), s, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer ss.Close()
			client := mcp.NewClient(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
			session, err := client.Connect(t.Context(), c, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			result, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: gallery.URI})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Contents) != 1 {
				t.Fatalf("contents: %d", len(result.Contents))
			}
			page := result.Contents[0].Text
			for _, part := range []struct{ name, tag string }{{"gallery.css", "style"}, {"gallery.js", "script"}} {
				source, err := os.ReadFile("../../../../frontend/mcp-apps/gallery/" + part.name)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(page, "<"+part.tag+">"+string(source)+"</"+part.tag+">") {
					t.Fatalf("%s is not fully inline", part.name)
				}
			}
			csp := "img-src blob:;"
			if withMedia {
				csp = "img-src blob: https://media.example.test;"
			}
			if !strings.Contains(page, csp) || !strings.Contains(page, "connect-src 'none'") || strings.Contains(page, "<!-- frontend:") {
				t.Fatal("assembled CSP or placeholders invalid")
			}
			raw, err := json.Marshal(result.Contents[0].Meta)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), `"connectDomains":[]`) || !strings.Contains(string(raw), `"blob:"`) {
				t.Fatalf("CSP metadata: %s", raw)
			}
			if withMedia && !strings.Contains(string(raw), "https://media.example.test") {
				t.Fatalf("missing media origin: %s", raw)
			}
		})
	}
}
