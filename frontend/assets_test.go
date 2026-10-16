package frontend

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

func fixture(t *testing.T) fstest.MapFS {
	t.Helper()
	root := fstest.MapFS{}
	for name, value := range map[string]string{
		"mcp-apps/gallery/index.html":     "<html><!-- frontend:style --><!-- frontend:script --></html>",
		"mcp-apps/gallery/gallery.css":    "body{margin:0}",
		"mcp-apps/gallery/gallery.js":     "console.log('fixture');",
		"pages/oauth/consent.html":        "<html><!-- frontend:style -->{{.ClientName}}</html>",
		"pages/oauth/consent.css":         "main{color:black}",
		"pages/login/layout.html":         "{{template \"content\" .}}",
		"pages/login/manual.html":         "manual",
		"pages/login/callback-relay.html": "callback",
		"pages/login/result.html":         "result",
		"pages/login/page.css":            "body{color:black}",
	} {
		write(t, root, name, value)
	}
	return root
}
func write(t *testing.T, root fstest.MapFS, name, value string) {
	t.Helper()
	root[name] = &fstest.MapFile{Data: []byte(value)}
}

func TestLoadCleanAndRepeat(t *testing.T) {
	root := fixture(t)
	for range 2 {
		got, err := load(root)
		if err != nil {
			t.Fatal(err)
		}
		if got.GalleryHTML != "<html><style>body{margin:0}</style><script>console.log('fixture');</script></html>" {
			t.Fatal(got.GalleryHTML)
		}
		if got.ConsentHTML != "<html><style>main{color:black}</style>{{.ClientName}}</html>" {
			t.Fatal(got.ConsentHTML)
		}
		for name, want := range map[string]string{"manual.html": "manual", "page.css": "body{color:black}"} {
			value, err := fs.ReadFile(got.Login, name)
			if err != nil || string(value) != want {
				t.Fatalf("%s: %q, %v", name, value, err)
			}
		}
	}
	if len(root) != 10 {
		t.Fatal("loading changed source files")
	}
}

func TestLoadRejectsInvalidSource(t *testing.T) {
	for _, name := range []string{"missing", "missing-style", "duplicate-style", "missing-script", "duplicate-script", "oauth-style"} {
		t.Run(name, func(t *testing.T) {
			root := fixture(t)
			switch name {
			case "missing":
				delete(root, "pages/login/result.html")
			case "missing-style":
				write(t, root, "mcp-apps/gallery/index.html", "<!-- frontend:script -->")
			case "duplicate-style":
				write(t, root, "mcp-apps/gallery/index.html", "<!-- frontend:style --><!-- frontend:style --><!-- frontend:script -->")
			case "missing-script":
				write(t, root, "mcp-apps/gallery/index.html", "<!-- frontend:style -->")
			case "duplicate-script":
				write(t, root, "mcp-apps/gallery/index.html", "<!-- frontend:style --><!-- frontend:script --><!-- frontend:script -->")
			case "oauth-style":
				write(t, root, "pages/oauth/consent.html", "{{.ClientName}}")
			}
			if _, err := load(root); err == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
}

func TestEmbeddedPagesContainOnlyPageSources(t *testing.T) {
	pages, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if pages.GalleryHTML == "" || pages.ConsentHTML == "" {
		t.Fatal("missing embedded pages")
	}
	names := []string{}
	if err := fs.WalkDir(sources, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			names = append(names, name)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	expected := fixture(t)
	if len(names) != len(expected) {
		t.Fatalf("unexpected embedded files: %v", names)
	}
	for _, name := range names {
		if _, ok := expected[name]; !ok {
			t.Fatalf("unexpected embedded source %s", name)
		}
	}
}
