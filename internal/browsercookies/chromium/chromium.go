// Package chromium 实现 Chrome 与 Edge（共享 Chromium cookie 格式）provider。
//
// 它从当前平台的用户数据目录发现 profile，用只读 sqlite3 查询 Cookies
// 数据库，并通过对应 OS 的秘密后端解密 Chromium cookie value。
package chromium

import (
	"context"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/sqliteio"
	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
)

const cookiesFile = "Cookies"

// selectCookiesSQL 是唯一允许执行的查询：按受约束的 host/name 精确匹配，
// 取 host_key、目标 cookie 的明文 value 与加密值（hex）；host_key 用于验证
// 现代 Chromium 数据库在加密明文前加入的域摘要。
const selectCookiesSQL = `SELECT host_key, value, hex(encrypted_value) FROM cookies WHERE (host_key = @h1 OR host_key = @h2) AND name = @n;`

func init() {
	browsercookies.Register("chrome", func() (browsercookies.Provider, error) { return newProvider("chrome") })
	browsercookies.Register("edge", func() (browsercookies.Provider, error) { return newProvider("edge") })
}

type kind int

const (
	kindChrome kind = iota
	kindEdge
)

func (k kind) rootDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return browserDataRoot(home, k)
}

type provider struct {
	name string
	kind kind
	root string
}

// newProvider 使用默认用户数据目录构造 provider。
func newProvider(name string) (*provider, error) {
	k := kindChrome
	if name == "edge" {
		k = kindEdge
	}
	return &provider{name: name, kind: k, root: k.rootDir()}, nil
}

func (p *provider) Name() string { return p.name }

func (p *provider) Close() error { return nil }

// DiscoverProfiles 扫描用户数据目录下包含 Cookies 文件的 profile 子目录。
// 不修改或启动浏览器；目录不存在时返回 ErrNotInstalled。
func (p *provider) DiscoverProfiles(ctx context.Context) ([]browsercookies.Profile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(p.root)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return nil, browsercookies.ErrPermissionDenied
		}
		return nil, browsercookies.ErrNotInstalled
	}
	var profiles []browsercookies.Profile
	for _, entry := range entries {
		if !entry.IsDir() || !browsercookies.ValidProfileID(entry.Name()) {
			continue
		}
		info, err := os.Stat(filepath.Join(p.root, entry.Name(), cookiesFile))
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				return nil, browsercookies.ErrPermissionDenied
			}
			continue
		}
		if info.IsDir() {
			continue
		}
		profiles = append(profiles, browsercookies.Profile{
			ID:   entry.Name(),
			Name: entry.Name(),
			Path: filepath.Join(p.root, entry.Name()),
		})
	}
	if len(profiles) == 0 {
		return nil, browsercookies.ErrNotInstalled
	}
	return profiles, nil
}

// Read 读取 profileID 下匹配 query 的 cookie secret。解密前查询已经收敛到
// 固定 host/name，且浏览器数据库通过 sqliteio 的只读参数查询。
func (p *provider) Read(ctx context.Context, query browsercookies.CookieQuery, profileID string) ([]credential.Secret, error) {
	if err := query.Valid(); err != nil {
		return nil, err
	}
	if !browsercookies.ValidProfileID(profileID) {
		return nil, browsercookies.ErrInvalidProfileID
	}
	cookiesPath := filepath.Join(p.root, profileID, cookiesFile)
	if _, err := os.Stat(cookiesPath); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return nil, browsercookies.ErrPermissionDenied
		}
		return nil, browsercookies.ErrDatabaseNotFound
	}
	params := map[string]string{
		"@h1": query.Host,
		"@h2": strings.TrimPrefix(query.Host, "."),
		"@n":  query.Name,
	}
	rows, err := sqliteio.Query(ctx, cookiesPath, selectCookiesSQL, params)
	if err != nil {
		return nil, err
	}
	return p.rowsToSecrets(ctx, rows)
}

func (p *provider) rowsToSecrets(ctx context.Context, rows [][]string) ([]credential.Secret, error) {
	secrets := make([]credential.Secret, 0, len(rows))
	for _, row := range rows {
		if len(row) < 3 {
			return nil, browsercookies.ErrQueryFailed
		}
		host := row[0]
		plain := row[1]
		encHex := strings.TrimSpace(row[2])
		if encHex != "" {
			enc, err := hex.DecodeString(encHex)
			if err != nil {
				return nil, browsercookies.ErrEncryptedFormatUnknown
			}
			value, err := p.decryptEncrypted(ctx, enc)
			if err != nil {
				return nil, err
			}
			secrets = append(secrets, credential.New(stripChromiumHostDigest(value, host)))
			continue
		}
		secrets = append(secrets, credential.New([]byte(plain)))
	}
	return secrets, nil
}
