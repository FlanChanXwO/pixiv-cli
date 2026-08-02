// Package firefox 实现 Firefox provider。
//
// 它从 profiles.ini 发现 profile，从 cookies.sqlite 读取明文 cookie；值看起来
// 加密（非 UTF-8）时返回明确分类。Firefox 未安装时 Discover 返回
// ErrNotInstalled，New 本身仍返回可用 provider。
package firefox

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/FlanChanXwO/pixiv-cli/internal/platform/browsercookies/core"
	"github.com/FlanChanXwO/pixiv-cli/internal/platform/browsercookies/sqliteio"
)

const (
	profilesIni = "profiles.ini"
	cookiesDB   = "cookies.sqlite"

	// selectCookiesSQL 是唯一允许执行的查询：按受约束的 host/name 精确匹配。
	selectCookiesSQL = `SELECT value FROM moz_cookies WHERE (host = @h1 OR host = @h2) AND name = @n;`
)

func init() {
	core.Register("firefox", func() (core.Provider, error) { return newProvider("") })
}

// defaultRoot 返回 Firefox 用户目录（macOS 路径；其他平台不存在时按未安装处理）。
func defaultRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support", "Firefox")
}

type provider struct {
	root string
}

// newProvider 构造 provider；root 为空时使用默认用户目录。
func newProvider(root string) (*provider, error) {
	if root == "" {
		root = defaultRoot()
	}
	return &provider{root: root}, nil
}

func (p *provider) Name() string { return "firefox" }

func (p *provider) Close() error { return nil }

// safeProfileID 校验 Firefox profile 目录名是安全 identifier：非空、非隐藏。
func safeProfileID(id string) bool {
	if id == "" || id == "." || id == ".." || strings.HasPrefix(id, ".") {
		return false
	}
	return filepath.Base(id) == id
}

// DiscoverProfiles 解析 profiles.ini 中的 [ProfileN] 节，只保留存在
// cookies.sqlite 的 profile。Firefox 未安装（根目录或 profiles.ini 缺失）时
// 返回 ErrNotInstalled。
func (p *provider) DiscoverProfiles(ctx context.Context) ([]core.Profile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.Open(filepath.Join(p.root, profilesIni))
	if err != nil {
		return nil, core.ErrNotInstalled
	}
	defer file.Close()
	profiles := parseProfilesIni(file, p.root)
	if len(profiles) == 0 {
		return nil, core.ErrNotInstalled
	}
	return profiles, nil
}

// parseProfilesIni 解析 profiles.ini；相对 Path 相对于 root 解析，绝对 Path 使用
// 其 basename 作为安全 identifier。
func parseProfilesIni(file *os.File, root string) []core.Profile {
	type iniSection struct {
		name       string
		path       string
		isRelative bool
	}
	var sections []iniSection
	current := -1
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sections = append(sections, iniSection{isRelative: true})
			current = len(sections) - 1
			continue
		}
		if current < 0 {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "Name":
			sections[current].name = value
		case "Path":
			sections[current].path = value
		case "IsRelative":
			sections[current].isRelative = value == "1"
		}
	}
	var profiles []core.Profile
	for _, s := range sections {
		if s.path == "" {
			continue
		}
		dir := s.path
		if s.isRelative {
			dir = filepath.Join(root, s.path)
		}
		id := filepath.Base(s.path)
		if !safeProfileID(id) {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, cookiesDB)); err != nil {
			continue
		}
		name := s.name
		if name == "" {
			name = id
		}
		profiles = append(profiles, core.Profile{ID: id, Name: name, Path: dir})
	}
	return profiles
}

// resolveProfile 通过 discovery 按 identifier 解析 profile 的绝对路径。
func (p *provider) resolveProfile(ctx context.Context, profileID string) (core.Profile, error) {
	profiles, err := p.DiscoverProfiles(ctx)
	if err != nil {
		return core.Profile{}, err
	}
	return core.SelectProfile(profiles, profileID)
}

// Read 读取 profileID 下匹配 query 的明文 cookie。加密值（非 UTF-8）返回
// ErrEncryptedCookieUnsupported 分类。
func (p *provider) Read(ctx context.Context, query core.CookieQuery, profileID string) ([]core.Secret, error) {
	if err := query.Valid(); err != nil {
		return nil, err
	}
	if !safeProfileID(profileID) {
		return nil, core.ErrInvalidProfileID
	}
	profile, err := p.resolveProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(profile.Path, cookiesDB)
	data, hooked, err := core.HookBytes("firefox", dbPath)
	if err != nil {
		return nil, err
	}
	path := dbPath
	if hooked {
		var cleanup func()
		path, cleanup, err = core.WriteTempSnapshot(data)
		if err != nil {
			return nil, err
		}
		defer cleanup()
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
	secrets := make([]core.Secret, 0, len(rows))
	for _, row := range rows {
		if len(row) < 1 {
			return nil, core.ErrQueryFailed
		}
		value := row[0]
		if looksEncrypted(value) {
			return nil, core.ErrEncryptedCookieUnsupported
		}
		secrets = append(secrets, core.NewSecret(value))
	}
	return secrets, nil
}

// looksEncrypted 用 UTF-8 合法性判断 cookie 值是否看起来已加密。
func looksEncrypted(value string) bool {
	return !utf8.ValidString(value)
}
