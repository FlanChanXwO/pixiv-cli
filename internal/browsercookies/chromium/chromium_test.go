package chromium

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
)

// skipIfNoSQLite3 在系统缺少 sqlite3 CLI 时跳过依赖它的测试。
func skipIfNoSQLite3(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 command-line tool not available")
	}
}

// buildFixtureDB 用 modernc.org/sqlite 构造 fixture SQLite 文件的字节。
func buildFixtureDB(t *testing.T, schema string, statements ...string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range append([]string{schema}, statements...) {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

const cookiesSchema = `CREATE TABLE cookies (
	host_key TEXT NOT NULL,
	name TEXT NOT NULL,
	value TEXT NOT NULL,
	encrypted_value BLOB NOT NULL DEFAULT ''
);`

func mustWriteFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverProfiles(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "Default", cookiesFile))
	mustWriteFile(t, filepath.Join(root, "Profile 1", cookiesFile))
	// 无 Cookies 文件的目录不是 profile。
	mustWriteFile(t, filepath.Join(root, "Cache", "data.txt"))

	p, err := newProvider("chrome", root)
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := p.DiscoverProfiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 {
		t.Fatalf("profiles = %+v, want 2", profiles)
	}
	ids := map[string]bool{}
	for _, pr := range profiles {
		ids[pr.ID] = true
		if pr.Path == "" {
			t.Fatalf("profile %q missing path", pr.ID)
		}
	}
	if !ids["Default"] || !ids["Profile 1"] {
		t.Fatalf("profiles missing expected IDs: %+v", ids)
	}
}

func TestDiscoverProfilesNotInstalled(t *testing.T) {
	p, err := newProvider("chrome", filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.DiscoverProfiles(context.Background()); !errors.Is(err, browsercookies.ErrNotInstalled) {
		t.Fatalf("err = %v, want ErrNotInstalled", err)
	}
	// Edge 同样未安装。
	pe, err := newProvider("edge", filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pe.DiscoverProfiles(context.Background()); !errors.Is(err, browsercookies.ErrNotInstalled) {
		t.Fatalf("edge err = %v, want ErrNotInstalled", err)
	}
}

func TestReadPlaintextCookie(t *testing.T) {
	skipIfNoSQLite3(t)
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "Default", cookiesFile))

	fixture := buildFixtureDB(t, cookiesSchema,
		`INSERT INTO cookies (host_key, name, value, encrypted_value) VALUES ('.fanbox.cc', 'FANBOXSESSID', 'plain-session', X'');`,
	)
	restore := browsercookies.SetProviderFixtureForTest("chrome", func(string) ([]byte, error) { return fixture, nil })
	defer restore()

	p, err := newProvider("chrome", root)
	if err != nil {
		t.Fatal(err)
	}
	secrets, err := p.Read(context.Background(), browsercookies.DefaultQuery, "Default")
	if err != nil {
		t.Fatal(err)
	}
	if len(secrets) != 1 || secrets[0].Value() != "plain-session" {
		t.Fatalf("secrets = %+v", secrets)
	}
	// 脱敏路径：格式化不泄露。
	if got := secrets[0].String(); got == "plain-session" {
		t.Fatal("Secret.String leaked value")
	}
}

func TestReadPlaintextHostVariant(t *testing.T) {
	skipIfNoSQLite3(t)
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "Default", cookiesFile))

	// 存储 host 无前导点（"fanbox.cc"），query ".fanbox.cc" 也应命中。
	fixture := buildFixtureDB(t, cookiesSchema,
		`INSERT INTO cookies (host_key, name, value, encrypted_value) VALUES ('fanbox.cc', 'FANBOXSESSID', 'no-dot-session', X'');`,
	)
	restore := browsercookies.SetProviderFixtureForTest("chrome", func(string) ([]byte, error) { return fixture, nil })
	defer restore()

	p, _ := newProvider("chrome", root)
	secrets, err := p.Read(context.Background(), browsercookies.DefaultQuery, "Default")
	if err != nil {
		t.Fatal(err)
	}
	if len(secrets) != 1 || secrets[0].Value() != "no-dot-session" {
		t.Fatalf("secrets = %+v", secrets)
	}
}

func TestReadModernEncryptedCookieHostDigest(t *testing.T) {
	skipIfNoSQLite3(t)
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "Default", cookiesFile))
	key := []byte("0123456789abcdef")
	host := ".fanbox.cc"
	blob := buildModernV10CBCBlob(t, key, host, []byte("modern-session"))
	fixture := buildFixtureDB(t, cookiesSchema,
		`INSERT INTO cookies (host_key, name, value, encrypted_value) VALUES ('`+host+`', 'FANBOXSESSID', '', X'`+hex.EncodeToString(blob)+`');`,
	)
	restore := browsercookies.SetProviderFixtureForTest("chrome", func(string) ([]byte, error) { return fixture, nil })
	defer restore()

	p, _ := newProvider("chrome", root)
	p.encryptionKeyOverride = func(context.Context) ([][]byte, error) { return [][]byte{key}, nil }
	secrets, err := p.Read(context.Background(), browsercookies.DefaultQuery, "Default")
	if err != nil {
		t.Fatal(err)
	}
	if len(secrets) != 1 || secrets[0].Value() != "modern-session" {
		t.Fatalf("secrets = %+v", secrets)
	}
}

// TestReadEncryptedClassification 验证损坏的短 blob 不会被误报成凭据缺失。
// Windows 的旧格式本身就是无长度约束的 DPAPI blob，因此由 DPAPI 返回其
// 平台解密错误；macOS/Linux 则在进入 secret backend 前返回格式错误。
func TestReadEncryptedClassification(t *testing.T) {
	skipIfNoSQLite3(t)
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "Default", cookiesFile))

	fixture := buildFixtureDB(t, cookiesSchema,
		`INSERT INTO cookies (host_key, name, value, encrypted_value) VALUES ('.fanbox.cc', 'FANBOXSESSID', '', X'DEADBEEF');`,
	)
	restore := browsercookies.SetProviderFixtureForTest("chrome", func(string) ([]byte, error) { return fixture, nil })
	defer restore()

	p, _ := newProvider("chrome", root)
	_, err := p.Read(context.Background(), browsercookies.DefaultQuery, "Default")
	if err == nil {
		t.Fatal("expected classification error")
	}
	want := browsercookies.ErrEncryptedFormatUnknown
	if runtime.GOOS == "windows" {
		want = browsercookies.ErrDPAPI
	}
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

// TestReadLockedDatabaseClassified 用现代 SQLite 锁验证浏览器运行时的
// "database is locked" 分类（真实路径、无 hook）。
func TestReadLockedDatabaseClassified(t *testing.T) {
	skipIfNoSQLite3(t)
	root := t.TempDir()
	dbPath := filepath.Join(root, "Default", cookiesFile)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(cookiesSchema); err != nil {
		t.Fatal(err)
	}
	// 持有 EXCLUSIVE 事务，模拟运行中的浏览器锁定数据库。
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), "BEGIN EXCLUSIVE"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		_ = conn.Close()
	}()

	p, _ := newProvider("chrome", root)
	if _, err := p.Read(context.Background(), browsercookies.DefaultQuery, "Default"); !errors.Is(err, browsercookies.ErrDatabaseLocked) {
		t.Fatalf("err = %v, want ErrDatabaseLocked", err)
	}
}

func TestReadInvalidQueryOrProfile(t *testing.T) {
	p, _ := newProvider("chrome", t.TempDir())
	if _, err := p.Read(context.Background(), browsercookies.CookieQuery{}, "Default"); !errors.Is(err, browsercookies.ErrQueryInvalid) {
		t.Fatalf("invalid query err = %v", err)
	}
	if _, err := p.Read(context.Background(), browsercookies.DefaultQuery, "../evil"); !errors.Is(err, browsercookies.ErrInvalidProfileID) {
		t.Fatalf("invalid profile err = %v", err)
	}
	if _, err := p.Read(context.Background(), browsercookies.DefaultQuery, ""); !errors.Is(err, browsercookies.ErrInvalidProfileID) {
		t.Fatalf("empty profile err = %v", err)
	}
}

func TestReadDatabaseNotFound(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "Default", cookiesFile))
	p, _ := newProvider("chrome", root)
	// 无 hook：真实路径不存在该 profile 的 Cookies（"Default" 存在占位，
	// 但换成不存在的 profile）。
	if _, err := p.Read(context.Background(), browsercookies.DefaultQuery, "Nope"); !errors.Is(err, browsercookies.ErrDatabaseNotFound) {
		t.Fatalf("err = %v, want ErrDatabaseNotFound", err)
	}
}
