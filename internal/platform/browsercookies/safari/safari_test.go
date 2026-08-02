package safari

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/platform/browsercookies/core"
)

// buildRecord 按 binarycookies cookie record 布局构造一条记录。
func buildRecord(c safariCookie) []byte {
	var buf bytes.Buffer
	buf.Write(make([]byte, 2)) // size 占位
	binary.Write(&buf, binary.BigEndian, uint16(0))
	writeLen := func(s string) {
		buf.WriteByte(byte(len(s)))
		buf.WriteString(s)
	}
	writeLen(c.domain)
	writeLen(c.name)
	writeLen(c.path)
	buf.WriteByte(byte(len(c.value)))
	buf.Write(c.value)
	buf.Write(make([]byte, 12)) // expiry + creation + modified
	flags := byte(0)
	if c.secure {
		flags |= 0x01
	}
	if c.httpOnly {
		flags |= 0x04
	}
	buf.WriteByte(flags)
	buf.WriteByte(0)
	record := buf.Bytes()
	binary.BigEndian.PutUint16(record[0:2], uint16(len(record)))
	return record
}

// buildBinaryCookies 构造一个单页 Cookies.binarycookies 文件字节。
func buildBinaryCookies(t *testing.T, cookies ...safariCookie) []byte {
	t.Helper()
	records := make([][]byte, 0, len(cookies))
	total := 0
	for _, c := range cookies {
		record := buildRecord(c)
		records = append(records, record)
		total += len(record)
	}
	pageSize := 16 + 4*len(cookies) + 4 + total
	page := make([]byte, pageSize)
	binary.BigEndian.PutUint32(page[0:4], 16) // headerSize
	binary.BigEndian.PutUint32(page[4:8], uint32(len(cookies)))
	binary.BigEndian.PutUint32(page[8:12], 16) // pageStart
	binary.BigEndian.PutUint32(page[12:16], uint32(pageSize))
	recordStart := 16 + 4*len(cookies) + 4
	for i, record := range records {
		// 表中偏移相对 pageStart（16）；解析端为 offset + pageStart。
		binary.BigEndian.PutUint32(page[16+4*i:16+4*i+4], uint32(recordStart-16))
		copy(page[recordStart:recordStart+len(record)], record)
		recordStart += len(record)
	}
	file := make([]byte, 0, 24+len(page))
	file = append(file, []byte("cook")...)
	var header [20]byte
	binary.BigEndian.PutUint32(header[0:4], 1) // numPages
	binary.BigEndian.PutUint32(header[4:8], uint32(len(page)))
	file = append(file, header[:8]...)
	file = append(file, page...)
	return file
}

func writePlaceholder(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDiscoverProfiles(t *testing.T) {
	root := t.TempDir()
	path := writePlaceholder(t, root, "Cookies.binarycookies")
	p, _ := newProvider([]string{path})
	profiles, err := p.DiscoverProfiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].ID != "Default" {
		t.Fatalf("profiles = %+v", profiles)
	}
}

func TestDiscoverNotInstalled(t *testing.T) {
	p, _ := newProvider([]string{filepath.Join(t.TempDir(), "missing.binarycookies")})
	if _, err := p.DiscoverProfiles(context.Background()); !errors.Is(err, core.ErrNotInstalled) {
		t.Fatalf("err = %v, want ErrNotInstalled", err)
	}
}

func TestReadPlaintextBinaryCookies(t *testing.T) {
	file := buildBinaryCookies(t, safariCookie{
		domain: ".fanbox.cc", name: "FANBOXSESSID", path: "/",
		value: []byte("sf-session"), secure: true, httpOnly: true,
	})
	restore := core.SetProviderFixtureForTest("safari", func(string) ([]byte, error) { return file, nil })
	defer restore()

	p, _ := newProvider([]string{filepath.Join(t.TempDir(), "Cookies.binarycookies")})
	secrets, err := p.Read(context.Background(), core.DefaultQuery, "Default")
	if err != nil {
		t.Fatal(err)
	}
	if len(secrets) != 1 || secrets[0].Value() != "sf-session" {
		t.Fatalf("secrets = %+v", secrets)
	}
}

func TestReadHostVariantAndNonMatching(t *testing.T) {
	file := buildBinaryCookies(t,
		safariCookie{domain: "fanbox.cc", name: "FANBOXSESSID", path: "/", value: []byte("no-dot")},
		safariCookie{domain: ".example.com", name: "OTHER", path: "/", value: []byte("x")},
	)
	restore := core.SetProviderFixtureForTest("safari", func(string) ([]byte, error) { return file, nil })
	defer restore()

	p, _ := newProvider([]string{filepath.Join(t.TempDir(), "Cookies.binarycookies")})
	secrets, err := p.Read(context.Background(), core.DefaultQuery, "Default")
	if err != nil {
		t.Fatal(err)
	}
	if len(secrets) != 1 || secrets[0].Value() != "no-dot" {
		t.Fatalf("secrets = %+v", secrets)
	}
}

func TestReadEncryptedClassified(t *testing.T) {
	file := buildBinaryCookies(t, safariCookie{
		domain: ".fanbox.cc", name: "FANBOXSESSID", path: "/",
		value: []byte{0x8F, 0x8F, 0x8F, 0x8F},
	})
	restore := core.SetProviderFixtureForTest("safari", func(string) ([]byte, error) { return file, nil })
	defer restore()

	p, _ := newProvider([]string{filepath.Join(t.TempDir(), "Cookies.binarycookies")})
	if _, err := p.Read(context.Background(), core.DefaultQuery, "Default"); !errors.Is(err, core.ErrEncryptedCookieUnsupported) {
		t.Fatalf("err = %v, want ErrEncryptedCookieUnsupported", err)
	}
}

func TestReadInvalidFormat(t *testing.T) {
	restore := core.SetProviderFixtureForTest("safari", func(string) ([]byte, error) {
		return []byte("not-a-binarycookies-file"), nil
	})
	defer restore()
	p, _ := newProvider([]string{filepath.Join(t.TempDir(), "Cookies.binarycookies")})
	if _, err := p.Read(context.Background(), core.DefaultQuery, "Default"); !errors.Is(err, core.ErrInvalidFormat) {
		t.Fatalf("err = %v, want ErrInvalidFormat", err)
	}
}

func TestReadDatabaseNotFound(t *testing.T) {
	p, _ := newProvider([]string{filepath.Join(t.TempDir(), "missing.binarycookies")})
	if _, err := p.Read(context.Background(), core.DefaultQuery, "Default"); !errors.Is(err, core.ErrDatabaseNotFound) {
		t.Fatalf("err = %v, want ErrDatabaseNotFound", err)
	}
	// 未知 profile。
	root := t.TempDir()
	writePlaceholder(t, root, "Cookies.binarycookies")
	p2, _ := newProvider([]string{filepath.Join(root, "Cookies.binarycookies")})
	if _, err := p2.Read(context.Background(), core.DefaultQuery, "Other"); !errors.Is(err, core.ErrProfileNotFound) {
		t.Fatalf("unknown profile err = %v", err)
	}
}
