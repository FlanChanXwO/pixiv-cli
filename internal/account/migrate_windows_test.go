//go:build windows

package account_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/account"
)

// Windows CI 实际只读导入旧库，覆盖盘符、反斜杠、空格与 URI 转义。
func TestOpenOnWindowsAcceptsForwardSlashFileURI(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "auth path #%")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join("testdata", "v1.2.1.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "pixiv-cli.db"), body, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := account.Open(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := store.ListPixiv(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || accounts[0].UserID != 42 {
		t.Fatal("legacy account was not imported")
	}
}
