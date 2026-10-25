// 发布身份与 GitHub Release 正文渲染：Version/Channel、六个 archive 目标与 archive name。
// runner 与 capability 等可变平台 metadata 只由 scripts/build/platforms.json 持有。
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	versionpkg "github.com/FlanChanXwO/pixiv-cli/internal/version"
)

// Target 是固定的 release 目标（GOOS/GOARCH 对）。
type Target struct {
	GOOS   string
	GOARCH string
}

// String 返回稳定 identity，例如 "linux/amd64"。
func (t Target) String() string {
	return t.GOOS + "/" + t.GOARCH
}

// FixedTargets 返回六个固定发布平台。
func FixedTargets() []Target {
	return []Target{
		{GOOS: "darwin", GOARCH: "amd64"},
		{GOOS: "darwin", GOARCH: "arm64"},
		{GOOS: "linux", GOARCH: "amd64"},
		{GOOS: "linux", GOARCH: "arm64"},
		{GOOS: "windows", GOARCH: "amd64"},
		{GOOS: "windows", GOARCH: "arm64"},
	}
}

// ArchiveName 返回 release archive 的固定文件名；darwin/linux 为 .tar.gz，Windows 为 .zip。
func ArchiveName(version string, target Target) string {
	extension := ".tar.gz"
	if target.GOOS == "windows" {
		extension = ".zip"
	}
	return "pixiv-cli_" + version + "_" + target.GOOS + "_" + target.GOARCH + extension
}

// ValidateVersion 要求 version 是不带前导 v 的 semantic version。
func ValidateVersion(version string) error {
	return versionpkg.Validate(version)
}

// Channel 返回 release 的稳定渠道名称。调用方必须先 ValidateVersion；build metadata
// 内合法的连字符不会被视为 prerelease 分隔符。
func Channel(version string) string {
	return versionpkg.Channel(version)
}

// NotesFromChangelog returns the release text after exactly one matching H1 heading.
func NotesFromChangelog(path, version string) ([]byte, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read changelog: %w", err)
	}
	header := regexp.MustCompile(`(?m)^# v` + regexp.QuoteMeta(version) + `(?:\s+[—-]\s+.+)?\s*$`)
	location := header.FindIndex(body)
	if location == nil {
		return nil, fmt.Errorf("changelog has no release heading for v%s", version)
	}
	if second := header.FindIndex(body[location[1]:]); second != nil {
		return nil, fmt.Errorf("changelog has more than one release heading for v%s", version)
	}
	notes := strings.Trim(internalNotePattern.ReplaceAllString(string(body[location[1]:]), ""), "\n")
	if notes == "" {
		return nil, fmt.Errorf("changelog release v%s has no release notes", version)
	}
	return []byte(notes + "\n"), nil
}

// BilingualBody keeps the language headings and separators stable for GitHub Release bodies.
func BilingualBody(english, chinese []byte) []byte {
	return []byte("# English\n\n" + strings.TrimSpace(string(english)) + "\n\n---\n\n# 简体中文\n\n" + strings.TrimSpace(string(chinese)) + "\n")
}
