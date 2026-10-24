package gallery_test

import (
	"crypto/sha256"
	"encoding/hex"
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
			var media *mediaproxy.Service
			if withMedia {
				store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
				if _, err := store.Init(t.Context(), false); err != nil {
					t.Fatal(err)
				}
				var err error
				media, err = mediaproxy.New("https://media.example.test/pixiv", store, nil)
				if err != nil {
					t.Fatal(err)
				}
				gallery.Register(server, media)
			} else {
				gallery.Register(server, nil)
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
			tool := &mcp.Tool{Name: "pixiv_show_gallery"}
			gallery.Attach(tool, media)
			viewURI, _ := tool.Meta["ui"].(map[string]any)["resourceUri"].(string)
			baseURI, _, _ := strings.Cut(viewURI, "/sha256-")
			mediaTool := &mcp.Tool{Name: "pixiv_artwork_media"}
			gallery.Attach(mediaTool, media)
			if mediaTool.Meta == nil || mediaTool.Meta["ui"] == nil {
				t.Fatal("complete media must have a host view instead of metadata-only output")
			}
			if mediaTool.Meta["ui"].(map[string]any)["resourceUri"] != viewURI {
				t.Fatal("complete media must reuse the current embedded view")
			}
			if viewURI == baseURI || !strings.HasPrefix(viewURI, baseURI+"/sha256-") {
				t.Fatalf("host UI must use a content-version cache key, got %q", viewURI)
			}
			result, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: baseURI})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Contents) != 1 {
				t.Fatalf("contents: %d", len(result.Contents))
			}
			page := result.Contents[0].Text
			digest := sha256.Sum256([]byte(page))
			if viewURI != baseURI+"/sha256-"+hex.EncodeToString(digest[:]) {
				t.Fatal("frontend and injected canonical origin must determine the host cache key")
			}
			for _, uri := range []string{viewURI, baseURI + "/sha256-" + strings.Repeat("0", 64)} {
				alias, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uri})
				if err != nil {
					t.Fatalf("current and historical view aliases remain readable: %v", err)
				}
				if len(alias.Contents) != 1 || alias.Contents[0].URI != uri || alias.Contents[0].Text != page {
					t.Fatalf("alias %q did not deliver the same current embedded view", uri)
				}
			}
			for _, revision := range []string{"invalid", strings.Repeat("A", 64)} {
				if _, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: baseURI + "/sha256-" + revision}); err == nil {
					t.Fatal("malformed revision aliases must not be readable")
				}
			}
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
