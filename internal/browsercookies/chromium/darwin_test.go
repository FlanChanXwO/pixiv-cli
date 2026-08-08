//go:build darwin

package chromium

import (
	"context"
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/core"
)

// TestReadEncryptedCookieWithInjectedKey 在 macOS 上端到端验证加密 cookie 读取：
// 注入 Keychain 密码避免调用真实 `security` 命令。
func TestReadEncryptedCookieWithInjectedKey(t *testing.T) {
	skipIfNoSQLite3(t)
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "Default", cookiesFile))

	key := deriveChromeKey([]byte("test-password"))
	value := []byte("fanbox-session12")
	blob := buildV10Blob(t, key, value)

	fixture := buildFixtureDB(t, cookiesSchema,
		`INSERT INTO cookies (host_key, name, value, encrypted_value) VALUES ('.fanbox.cc', 'FANBOXSESSID', '', X'`+hex.EncodeToString(blob)+`');`,
	)
	restore := core.SetProviderFixtureForTest("chrome", func(string) ([]byte, error) { return fixture, nil })
	defer restore()

	p, err := newProvider("chrome", root)
	if err != nil {
		t.Fatal(err)
	}
	p.keychainKeyOverride = func(context.Context) ([]byte, error) { return key, nil }

	secrets, err := p.Read(context.Background(), core.DefaultQuery, "Default")
	if err != nil {
		t.Fatal(err)
	}
	if len(secrets) != 1 || secrets[0].Value() != string(value) {
		t.Fatalf("secrets = %+v", secrets)
	}
}

// TestReadEncryptedKeychainItemNotFound 验证 Keychain item 缺失的分类错误。
func TestReadEncryptedKeychainItemNotFound(t *testing.T) {
	skipIfNoSQLite3(t)
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "Default", cookiesFile))

	blob := buildV10Blob(t, deriveChromeKey([]byte("x")), []byte("fanbox-session12"))
	fixture := buildFixtureDB(t, cookiesSchema,
		`INSERT INTO cookies (host_key, name, value, encrypted_value) VALUES ('.fanbox.cc', 'FANBOXSESSID', '', X'`+hex.EncodeToString(blob)+`');`,
	)
	restore := core.SetProviderFixtureForTest("chrome", func(string) ([]byte, error) { return fixture, nil })
	defer restore()

	p, _ := newProvider("chrome", root)
	// 注入失败：模拟 Keychain item 不存在，验证分类错误且不泄露任何内容。
	p.keychainKeyOverride = func(context.Context) ([]byte, error) { return nil, core.ErrKeychainItemNotFound }
	_, err := p.Read(context.Background(), core.DefaultQuery, "Default")
	if !errors.Is(err, core.ErrKeychainItemNotFound) {
		t.Fatalf("err = %v, want ErrKeychainItemNotFound", err)
	}
}
