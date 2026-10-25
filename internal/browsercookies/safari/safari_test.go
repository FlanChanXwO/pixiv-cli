package safari

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
	"github.com/stretchr/testify/require"
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
	return buildBinaryCookiesPages(t, cookies)
}

func buildBinaryCookiesPages(t *testing.T, pages ...[]safariCookie) []byte {
	t.Helper()
	encodedPages := make([][]byte, 0, len(pages))
	for _, cookies := range pages {
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
		encodedPages = append(encodedPages, page)
	}
	var numPages [4]byte
	binary.BigEndian.PutUint32(numPages[:], uint32(len(encodedPages)))
	file := make([]byte, 0)
	file = append(file, []byte("cook")...)
	file = append(file, numPages[:]...)
	for _, page := range encodedPages {
		var pageSize [4]byte
		binary.BigEndian.PutUint32(pageSize[:], uint32(len(page)))
		file = append(file, pageSize[:]...)
		file = append(file, page...)
	}
	return file
}

func TestParseBinaryCookiesDoesNotTruncatePages(t *testing.T) {
	pages := make([][]safariCookie, 0, 5)
	for i := range 5 {
		pages = append(pages, []safariCookie{{domain: ".fanbox.cc", name: "FANBOXSESSID", value: []byte{byte('a' + i)}}})
	}
	cookies, err := parseBinaryCookies(buildBinaryCookiesPages(t, pages...))
	if err != nil {
		t.Fatal(err)
	}
	if len(cookies) != 5 {
		t.Fatalf("cookies = %d, want 5 pages", len(cookies))
	}
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
	t.Run("installed", func(t *testing.T) {
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
	})
	t.Run("not installed", func(t *testing.T) {
		p, _ := newProvider([]string{filepath.Join(t.TempDir(), "missing.binarycookies")})
		if _, err := p.DiscoverProfiles(context.Background()); !errors.Is(err, browsercookies.ErrNotInstalled) {
			t.Fatalf("err = %v, want ErrNotInstalled", err)
		}
	})
}

func TestReadClassifiesCookiesAndProfiles(t *testing.T) {
	for _, test := range []struct {
		name     string
		file     []byte
		profile  string
		settings []string
		value    string
		wantErr  error
	}{
		{
			name:    "plaintext",
			file:    buildBinaryCookies(t, safariCookie{domain: ".fanbox.cc", name: "FANBOXSESSID", path: "/", value: []byte("sf-session"), secure: true, httpOnly: true}),
			profile: "Default",
			value:   "sf-session",
		},
		{
			name: "bare host matches and unrelated cookie is ignored",
			file: buildBinaryCookies(t,
				safariCookie{domain: "fanbox.cc", name: "FANBOXSESSID", path: "/", value: []byte("no-dot")},
				safariCookie{domain: ".example.com", name: "OTHER", path: "/", value: []byte("x")},
			),
			profile: "Default",
			value:   "no-dot",
		},
		{
			name:    "encrypted value classified",
			file:    buildBinaryCookies(t, safariCookie{domain: ".fanbox.cc", name: "FANBOXSESSID", path: "/", value: []byte{0x8F, 0x8F, 0x8F, 0x8F}}),
			profile: "Default",
			wantErr: browsercookies.ErrEncryptedCookieUnsupported,
		},
		{
			name:    "invalid format",
			file:    []byte("not-a-binarycookies-file"),
			profile: "Default",
			wantErr: browsercookies.ErrInvalidFormat,
		},
		{
			name:    "database not found",
			profile: "Default",
			wantErr: browsercookies.ErrDatabaseNotFound,
		},
		{
			name:    "unknown profile",
			file:    buildBinaryCookies(t),
			profile: "Other",
			wantErr: browsercookies.ErrProfileNotFound,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Cookies.binarycookies")
			if test.file != nil {
				require.NoError(t, os.WriteFile(path, test.file, 0o600))
			}
			p, _ := newProvider([]string{path})
			secrets, err := p.Read(context.Background(), browsercookies.DefaultQuery, test.profile)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, secrets, 1)
			require.Equal(t, test.value, secrets[0].Value())
		})
	}
}
