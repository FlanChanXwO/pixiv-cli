package sqliteio_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/sqliteio"
	_ "modernc.org/sqlite"
)

func TestQueryReturnsPlaintextAndTrailingEmptyBlobColumn(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 command-line tool not available")
	}
	path := filepath.Join(t.TempDir(), "cookies.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE cookies (host_key TEXT, name TEXT, value TEXT, encrypted_value BLOB NOT NULL DEFAULT '');`,
		`INSERT INTO cookies (host_key, name, value, encrypted_value) VALUES ('.fanbox.cc', 'FANBOXSESSID', 'plain-session', X'');`,
	} {
		if _, err := db.Exec(statement); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	rows, err := sqliteio.Query(
		context.Background(),
		path,
		`SELECT host_key, value, hex(encrypted_value) FROM cookies WHERE (host_key = @h1 OR host_key = @h2) AND name = @n;`,
		map[string]string{"@h1": ".fanbox.cc", "@h2": "fanbox.cc", "@n": "FANBOXSESSID"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0]) != 3 {
		t.Fatalf("rows shape = %#v, want one row with three columns", rows)
	}
	if got, want := rows[0], []string{".fanbox.cc", "plain-session", ""}; !slices.Equal(got, want) {
		t.Fatalf("row = %#v, want %#v", got, want)
	}
}

func TestQueryForcesSingleNewlineForCSV(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("synthetic sqlite3 helper uses POSIX printf; Windows is covered by browser evidence")
	}
	dir := t.TempDir()
	command := filepath.Join(dir, "sqlite3")
	contents := `#!/bin/sh
expected_newline='
'
if [ "$4" = "-newline" ] && [ "$5" = "$expected_newline" ]; then
  printf '.fanbox.cc,plain-session,""\r\n'
else
  printf '.fanbox.cc,plain-session,""\r\r\n'
fi
`
	if err := os.WriteFile(command, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	rows, err := sqliteio.Query(
		context.Background(),
		filepath.Join(dir, "Cookies"),
		"SELECT host_key, value, hex(encrypted_value) FROM cookies;",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".fanbox.cc", "plain-session", ""}
	if len(rows) != 1 || !slices.Equal(rows[0], want) {
		t.Fatalf("rows = %#v, want %#v", rows, [][]string{want})
	}
}

func TestQueryMapsPermissionFailureWithoutLeakingCommandOutput(t *testing.T) {
	dir := t.TempDir()
	command := filepath.Join(dir, "sqlite3")
	contents := "#!/bin/sh\nprintf '%s\\n' 'permission denied: fixture-secret' >&2\nexit 1\n"
	if runtime.GOOS == "windows" {
		command += ".cmd"
		contents = "@echo permission denied: fixture-secret 1>&2\r\n@exit /b 1\r\n"
	}
	if err := os.WriteFile(command, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)

	_, err := sqliteio.Query(context.Background(), filepath.Join(dir, "Cookies"), "SELECT 1", nil)
	if !errors.Is(err, browsercookies.ErrPermissionDenied) {
		t.Fatalf("err = %v, want ErrPermissionDenied", err)
	}
	if err.Error() == "permission denied: fixture-secret" {
		t.Fatal("sqlite3 stderr leaked into error")
	}
}
