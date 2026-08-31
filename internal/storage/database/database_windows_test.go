//go:build windows

package database

import (
	"strings"
	"testing"
)

func TestDSNWithPragmasUsesSlashSeparatedFileURI(t *testing.T) {
	dsn := dsnWithPragmas(`C:\Users\runner\AppData\Local\pixiv-cli.db`)
	if !strings.HasPrefix(dsn, "file:///C:/Users/runner/AppData/Local/pixiv-cli.db?") {
		t.Fatalf("dsnWithPragmas() = %q, want a slash-separated absolute file URI", dsn)
	}
	if strings.Contains(dsn, `\\`) || strings.Contains(dsn, "%5C") {
		t.Fatalf("dsnWithPragmas() retained Windows path separators: %q", dsn)
	}
}
