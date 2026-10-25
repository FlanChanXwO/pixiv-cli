//go:build windows

package database

import (
	"context"
	"testing"

	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
)

// Windows CI 实际打开数据库，覆盖盘符、反斜杠、空格与 TMP 路径。
func TestOpenOnWindowsAcceptsForwardSlashFileURI(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SavePixivCredential(context.Background(), accountpixiv.New(1, "user", []byte("token"))); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetPixiv(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
}
