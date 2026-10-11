package frontendassets_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/frontendassets"
)

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, value := range map[string]string{
		"frontend/mcp-apps/gallery/index.html":     "<html><!-- frontend:style --><!-- frontend:script --></html>",
		"frontend/mcp-apps/gallery/gallery.css":    "body{margin:0}",
		"frontend/mcp-apps/gallery/gallery.js":     "console.log('fixture');",
		"frontend/pages/oauth/consent.html":        "<html><!-- frontend:style -->{{.ClientName}}</html>",
		"frontend/pages/oauth/consent.css":         "main{color:black}",
		"frontend/pages/login/layout.html":         "{{template \"content\" .}}",
		"frontend/pages/login/manual.html":         "manual",
		"frontend/pages/login/callback-relay.html": "callback",
		"frontend/pages/login/result.html":         "result",
		"frontend/pages/login/page.css":            "body{color:black}",
	} {
		write(t, root, name, value)
	}
	return root
}
func write(t *testing.T, root, name, value string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateCleanAndRepeat(t *testing.T) {
	root := fixture(t)
	if err := frontendassets.Generate(root, false); err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{
		"internal/mcpserver/pixiv/gallery/generated/gallery.html":                              "<html><style>body{margin:0}</style><script>console.log('fixture');</script></html>",
		"internal/mcpserver/auth/generated/consent.html":                                       "<html><style>main{color:black}</style>{{.ClientName}}</html>",
		"internal/services/pixiv/account/loginrelay/loginpage/generated/templates/manual.html": "manual",
		"internal/services/pixiv/account/loginrelay/loginpage/generated/page.css":              "body{color:black}",
	}
	for repeat := 0; repeat < 2; repeat++ {
		for name, want := range expected {
			got, err := os.ReadFile(filepath.Join(root, name))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != want {
				t.Errorf("%s: got %q want %q", name, got, want)
			}
		}
		if err := frontendassets.Generate(root, false); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGenerateInvalidSourceDoesNotWrite(t *testing.T) {
	for _, name := range []string{"missing", "missing-style", "duplicate-style", "missing-script", "duplicate-script", "oauth-style"} {
		t.Run(name, func(t *testing.T) {
			root := fixture(t)
			switch name {
			case "missing":
				if err := os.Remove(filepath.Join(root, "frontend/pages/login/result.html")); err != nil {
					t.Fatal(err)
				}
			case "missing-style":
				write(t, root, "frontend/mcp-apps/gallery/index.html", "<!-- frontend:script -->")
			case "duplicate-style":
				write(t, root, "frontend/mcp-apps/gallery/index.html", "<!-- frontend:style --><!-- frontend:style --><!-- frontend:script -->")
			case "missing-script":
				write(t, root, "frontend/mcp-apps/gallery/index.html", "<!-- frontend:style -->")
			case "duplicate-script":
				write(t, root, "frontend/mcp-apps/gallery/index.html", "<!-- frontend:style --><!-- frontend:script --><!-- frontend:script -->")
			case "oauth-style":
				write(t, root, "frontend/pages/oauth/consent.html", "{{.ClientName}}")
			}
			if err := frontendassets.Generate(root, false); err == nil {
				t.Fatal("invalid source accepted")
			}
			if _, err := os.Stat(filepath.Join(root, "internal")); !os.IsNotExist(err) {
				t.Fatalf("invalid input wrote output: %v", err)
			}
		})
	}
}

func TestCheckIsReadOnly(t *testing.T) {
	root := fixture(t)
	if err := frontendassets.Generate(root, true); err == nil {
		t.Fatal("missing outputs accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "internal")); !os.IsNotExist(err) {
		t.Fatalf("check wrote output: %v", err)
	}
	if err := frontendassets.Generate(root, false); err != nil {
		t.Fatal(err)
	}
	if err := frontendassets.Generate(root, true); err != nil {
		t.Fatal(err)
	}
	name := "internal/mcpserver/pixiv/gallery/generated/gallery.html"
	write(t, root, name, "stale")
	if err := frontendassets.Generate(root, true); err == nil {
		t.Fatal("stale output accepted")
	}
	got, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "stale" {
		t.Fatal("check rewrote stale output")
	}
}
