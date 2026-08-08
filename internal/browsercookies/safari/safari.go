// Package safari 实现 Safari provider。
//
// 它解析 Cookies.binarycookies 二进制格式，读取明文 cookie；值看起来加密
// （非 UTF-8）时返回明确分类。macOS 上 Safari 未安装时 Discover 返回
// ErrNotInstalled。
package safari

import (
	"context"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/core"
)

func init() {
	core.Register("safari", func() (core.Provider, error) { return newProvider(nil) })
}

// defaultCookiePaths 返回候选 Cookies.binarycookies 路径（新容器 + legacy）。
func defaultCookiePaths() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, "Library", "Containers", "com.apple.Safari", "Data", "Library", "Cookies", "Cookies.binarycookies"),
		filepath.Join(home, "Library", "Cookies", "Cookies.binarycookies"),
	}
}

type provider struct {
	paths []string
}

// newProvider 构造 provider；paths 为空时使用默认路径。
func newProvider(paths []string) (*provider, error) {
	if len(paths) == 0 {
		paths = defaultCookiePaths()
	}
	return &provider{paths: paths}, nil
}

func (p *provider) Name() string { return "safari" }

func (p *provider) Close() error { return nil }

func (p *provider) findCookieFile() (string, error) {
	for _, path := range p.paths {
		info, err := os.Stat(path)
		if err == nil {
			if !info.IsDir() {
				return path, nil
			}
			continue
		}
		if errors.Is(err, fs.ErrPermission) {
			return "", core.ErrPermissionDenied
		}
	}
	return "", nil
}

func (p *provider) firstCandidatePath() string {
	if len(p.paths) == 0 {
		return ""
	}
	return p.paths[0]
}

// DiscoverProfiles 返回单个 "Default" profile；找不到 cookie 文件时返回
// ErrNotInstalled。
func (p *provider) DiscoverProfiles(ctx context.Context) ([]core.Profile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := p.findCookieFile()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, core.ErrNotInstalled
	}
	return []core.Profile{{ID: "Default", Name: "Default", Path: path}}, nil
}

// Read 读取匹配 query 的明文 cookie。加密值（非 UTF-8）返回
// ErrEncryptedCookieUnsupported 分类。
func (p *provider) Read(ctx context.Context, query core.CookieQuery, profileID string) ([]core.Secret, error) {
	if err := query.Valid(); err != nil {
		return nil, err
	}
	if profileID != "Default" {
		return nil, core.ErrProfileNotFound
	}
	path, err := p.findCookieFile()
	if err != nil {
		return nil, err
	}
	if path == "" {
		path = p.firstCandidatePath()
		if path == "" {
			return nil, core.ErrDatabaseNotFound
		}
	}
	data, hooked, err := core.HookBytes("safari", path)
	if err != nil {
		return nil, err
	}
	if !hooked {
		data, err = os.ReadFile(path)
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				return nil, core.ErrPermissionDenied
			}
			return nil, core.ErrDatabaseNotFound
		}
	}
	cookies, err := parseBinaryCookies(data)
	if err != nil {
		return nil, err
	}
	secrets := make([]core.Secret, 0)
	for _, c := range cookies {
		if !cookieMatches(c, query) {
			continue
		}
		if !utf8.Valid(c.value) {
			return nil, core.ErrEncryptedCookieUnsupported
		}
		secrets = append(secrets, core.NewSecret(string(c.value)))
	}
	return secrets, nil
}

func cookieMatches(c safariCookie, query core.CookieQuery) bool {
	if c.name != query.Name {
		return false
	}
	if c.domain == query.Host {
		return true
	}
	// 无前导点匹配：query ".fanbox.cc" ↔ 存储 "fanbox.cc"。
	return strings.TrimPrefix(c.domain, ".") == strings.TrimPrefix(query.Host, ".")
}

type safariCookie struct {
	domain   string
	name     string
	path     string
	value    []byte
	secure   bool
	httpOnly bool
}

// parseBinaryCookies 解析 Safari Cookies.binarycookies 二进制格式。
func parseBinaryCookies(data []byte) ([]safariCookie, error) {
	if len(data) < 8 || string(data[:4]) != "cook" {
		return nil, core.ErrInvalidFormat
	}
	numPages := int(binary.BigEndian.Uint32(data[4:8]))
	if numPages < 1 {
		return nil, nil
	}
	// 先验证页目录完整，再按文件声明的页数解析；不能用无依据的页数
	// 上限静默丢弃合法 cookie。
	if numPages > (len(data)-8)/4 {
		return nil, core.ErrInvalidFormat
	}
	var cookies []safariCookie
	off := 8
	for range numPages {
		pageSize := int(binary.BigEndian.Uint32(data[off : off+4]))
		off += 4
		if pageSize < 16 || off+pageSize > len(data) {
			return nil, core.ErrInvalidFormat
		}
		page := data[off : off+pageSize]
		off += pageSize
		pageCookies, err := parsePage(page)
		if err != nil {
			return nil, err
		}
		cookies = append(cookies, pageCookies...)
	}
	return cookies, nil
}

func parsePage(page []byte) ([]safariCookie, error) {
	if len(page) < 16 {
		return nil, core.ErrInvalidFormat
	}
	headerSize := int(binary.BigEndian.Uint32(page[0:4]))
	numCookies := int(binary.BigEndian.Uint32(page[4:8]))
	pageStart := int(binary.BigEndian.Uint32(page[8:12]))
	if headerSize < 16 || pageStart < 16 || numCookies < 0 || headerSize+4*numCookies > len(page) {
		return nil, core.ErrInvalidFormat
	}
	offsets := make([]int, 0, numCookies)
	for i := range numCookies {
		raw := int(binary.BigEndian.Uint32(page[headerSize+4*i : headerSize+4*i+4]))
		offsets = append(offsets, raw+pageStart)
	}
	var cookies []safariCookie
	for _, offset := range offsets {
		if offset < 0 || offset >= len(page) {
			return nil, core.ErrInvalidFormat
		}
		c, err := parseCookie(page[offset:])
		if err != nil {
			return nil, err
		}
		cookies = append(cookies, c)
	}
	return cookies, nil
}

func parseCookie(record []byte) (safariCookie, error) {
	if len(record) < 4 {
		return safariCookie{}, core.ErrInvalidFormat
	}
	size := int(binary.BigEndian.Uint16(record[0:2]))
	if size < 4 || size > len(record) {
		return safariCookie{}, core.ErrInvalidFormat
	}
	record = record[:size]
	pos := 4 // 跳过 size(2) + version(2)
	read := func() ([]byte, error) {
		if pos >= len(record) {
			return nil, core.ErrInvalidFormat
		}
		n := int(record[pos])
		pos++
		if pos+n > len(record) {
			return nil, core.ErrInvalidFormat
		}
		b := record[pos : pos+n]
		pos += n
		return b, nil
	}
	domain, err := read()
	if err != nil {
		return safariCookie{}, err
	}
	name, err := read()
	if err != nil {
		return safariCookie{}, err
	}
	path, err := read()
	if err != nil {
		return safariCookie{}, err
	}
	value, err := read()
	if err != nil {
		return safariCookie{}, err
	}
	// value 之后：expiry(4) + creation(4) + modified(4) + flags(1) + unknown(1)。
	flagsPos := pos + 12
	if flagsPos >= len(record) {
		return safariCookie{}, core.ErrInvalidFormat
	}
	flags := record[flagsPos]
	return safariCookie{
		domain:   string(domain),
		name:     string(name),
		path:     string(path),
		value:    value,
		secure:   flags&0x01 != 0,
		httpOnly: flags&0x04 != 0,
	}, nil
}
