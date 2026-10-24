package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveDirectoryPaths(t *testing.T) {
	for _, format := range []struct {
		name string
		read func(string) (map[string]string, error)
	}{
		{"tar.gz", readTarGzMembers},
		{"zip", readZIPMembers},
	} {
		for _, test := range []struct {
			name   string
			unsafe bool
		}{
			{name: "."},
			{name: "./"},
			{name: "third_party/licenses"},
			{name: "third_party/licenses/"},
			{name: "./third_party/licenses/"},
			{name: "../outside/", unsafe: true},
			{name: "third_party/../../outside/", unsafe: true},
			{name: "third_party/..", unsafe: true},
			{name: "..", unsafe: true},
			{name: "/outside/", unsafe: true},
			{name: "\\outside\\", unsafe: true},
			{name: "third_party\\licenses\\", unsafe: true},
			{name: "C:/outside/", unsafe: true},
			{name: "c:outside/", unsafe: true},
			{name: "//server/share/", unsafe: true},
		} {
			t.Run(format.name+"/"+test.name, func(t *testing.T) {
				path := directoryArchive(t, format.name, test.name)
				members, err := format.read(path)
				if test.unsafe {
					if err == nil {
						t.Fatalf("unsafe directory %q was accepted", test.name)
					}
					return
				}
				if err != nil {
					t.Fatalf("read safe directory %q: %v", test.name, err)
				}
				if len(members) != 1 || members["pixiv"] == "" {
					t.Fatalf("regular members = %v, want only pixiv", members)
				}
			})
		}
	}
}

func directoryArchive(t *testing.T, format, directory string) string {
	t.Helper()
	var buffer bytes.Buffer
	body := []byte("synthetic binary")
	switch format {
	case "tar.gz":
		compressed := gzip.NewWriter(&buffer)
		writer := tar.NewWriter(compressed)
		if err := writer.WriteHeader(&tar.Header{Name: "pixiv", Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(body); err != nil {
			t.Fatal(err)
		}
		if err := writer.WriteHeader(&tar.Header{Name: directory, Mode: 0o755, Typeflag: tar.TypeDir}); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		if err := compressed.Close(); err != nil {
			t.Fatal(err)
		}
	case "zip":
		writer := zip.NewWriter(&buffer)
		member, err := writer.Create("pixiv")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write(body); err != nil {
			t.Fatal(err)
		}
		header := &zip.FileHeader{Name: directory}
		header.SetMode(os.ModeDir | 0o755)
		if _, err := writer.CreateHeader(header); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unsupported test format %q", format)
	}
	path := filepath.Join(t.TempDir(), "archive."+format)
	if err := os.WriteFile(path, buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
