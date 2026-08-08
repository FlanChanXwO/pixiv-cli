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

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/core"
	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/sqliteio"
)

const cookiesFile = "Cookies"

// selectCookiesSQL 是唯一允许执行的查询：按受约束的 host/name 精确匹配，
// 只取目标 cookie 的明文 value 与加密值（hex）。
const selectCookiesSQL = `SELECT value, hex(encrypted_value) FROM cookies WHERE (host_key = @h1 OR host_key = @h2) AND name = @n;`

func init() {
	core.Register("chrome", func() (core.Provider, error) { return newProvider("chrome", "") })
	core.Register("edge", func() (core.Provider, error) { return newProvider("edge", "") })
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
	// keychainKeyOverride 仅供测试注入 Keychain 密码；nil 时走真实 Keychain。
	keychainKeyOverride func(ctx context.Context) ([]byte, error)
	// encryptionKeyOverride 仅供跨平台 crypto fixture 注入已经解包的 AES key。
	// 生产路径始终走平台秘密后端或 DPAPI。
	encryptionKeyOverride func(ctx context.Context) ([][]byte, error)
}

// newProvider 构造 provider；root 为空时使用默认用户数据目录。
func newProvider(name, root string) (*provider, error) {
	k := kindChrome
	if name == "edge" {
		k = kindEdge
	}
	if root == "" {
		root = k.rootDir()
	}
	return &provider{name: name, kind: k, root: root}, nil
}

func (p *provider) Name() string { return p.name }

func (p *provider) Close() error { return nil }

// safeProfileID 校验 profile 目录名是安全 identifier：非空、非隐藏、无路径分隔。
func safeProfileID(id string) bool {
	if id == "" || id == "." || id == ".." || strings.HasPrefix(id, ".") {
		return false
	}
	return filepath.Base(id) == id
}

// DiscoverProfiles 扫描用户数据目录下包含 Cookies 文件的 profile 子目录。
// 不修改或启动浏览器；目录不存在时返回 ErrNotInstalled。
func (p *provider) DiscoverProfiles(ctx context.Context) ([]core.Profile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(p.root)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return nil, core.ErrPermissionDenied
		}
		return nil, core.ErrNotInstalled
	}
	var profiles []core.Profile
	for _, entry := range entries {
		if !entry.IsDir() || !safeProfileID(entry.Name()) {
			continue
		}
		info, err := os.Stat(filepath.Join(p.root, entry.Name(), cookiesFile))
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				return nil, core.ErrPermissionDenied
			}
			continue
		}
		if info.IsDir() {
			continue
		}
		profiles = append(profiles, core.Profile{
			ID:   entry.Name(),
			Name: entry.Name(),
			Path: filepath.Join(p.root, entry.Name()),
		})
	}
	if len(profiles) == 0 {
		return nil, core.ErrNotInstalled
	}
	return profiles, nil
}

// Read 读取 profileID 下匹配 query 的 cookie secret。解密前查询已经收敛到
// 固定 host/name，且浏览器数据库通过 sqliteio 的只读参数查询。
func (p *provider) Read(ctx context.Context, query core.CookieQuery, profileID string) ([]core.Secret, error) {
	if err := query.Valid(); err != nil {
		return nil, err
	}
	if !safeProfileID(profileID) {
		return nil, core.ErrInvalidProfileID
	}
	cookiesPath := filepath.Join(p.root, profileID, cookiesFile)
	data, hooked, err := core.HookBytes(p.name, cookiesPath)
	if err != nil {
		return nil, err
	}
	path := cookiesPath
	if hooked {
		var cleanup func()
		path, cleanup, err = core.WriteTempSnapshot(data)
		if err != nil {
			return nil, err
		}
		defer cleanup()
	} else if _, err := os.Stat(cookiesPath); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return nil, core.ErrPermissionDenied
		}
		return nil, core.ErrDatabaseNotFound
	}
	params := map[string]string{
		"@h1": query.Host,
		"@h2": strings.TrimPrefix(query.Host, "."),
		"@n":  query.Name,
	}
	rows, err := sqliteio.Query(ctx, path, selectCookiesSQL, params)
	if err != nil {
		return nil, err
	}
	snapshot, err := p.rowsToSnapshot(ctx, query, profileID, rows)
	if err != nil {
		return nil, err
	}
	secrets := make([]core.Secret, 0, len(snapshot.Cookies))
	for _, c := range snapshot.Cookies {
		secrets = append(secrets, c.Value)
	}
	return secrets, nil
}

func (p *provider) rowsToSnapshot(ctx context.Context, query core.CookieQuery, profileID string, rows [][]string) (core.Snapshot, error) {
	snap := core.Snapshot{ProfileID: profileID, Cookies: []core.Cookie{}}
	for _, row := range rows {
		if len(row) < 2 {
			return snap, core.ErrQueryFailed
		}
		plain := row[0]
		encHex := strings.TrimSpace(row[1])
		if encHex != "" {
			enc, err := hex.DecodeString(encHex)
			if err != nil {
				return snap, core.ErrEncryptedFormatUnknown
			}
			value, err := p.decryptEncrypted(ctx, enc)
			if err != nil {
				return snap, err
			}
			snap.Cookies = append(snap.Cookies, core.Cookie{
				Name:  query.Name,
				Value: core.NewSecret(string(value)),
			})
			continue
		}
		snap.Cookies = append(snap.Cookies, core.Cookie{
			Name:  query.Name,
			Value: core.NewSecret(plain),
		})
	}
	return snap, nil
}
