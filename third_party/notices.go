// Package thirdparty carries dependency notices with standalone executables.
package thirdparty

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
)

//go:embed all:licenses
var notices embed.FS

// WriteNotices writes the complete embedded third-party notices in path order.
func WriteNotices(w io.Writer) error {
	return fs.WalkDir(notices, "licenses", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		body, err := notices.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "\n## %s\n\n", path); err != nil {
			return err
		}
		_, err = w.Write(body)
		return err
	})
}
