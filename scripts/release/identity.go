package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Target 标识一个固定的 release 目标，由 GOOS 与 GOARCH 组成。
type Target struct {
	GOOS   string
	GOARCH string
}

// String 返回 "GOOS/GOARCH" 形式的稳定身份，例如 "linux/amd64"。
func (t Target) String() string {
	return t.GOOS + "/" + t.GOARCH
}

// FixedTargets 返回六个发布平台，顺序即 release 归档与 checksums 的固定顺序。
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

// ArchiveName 生成 release archive 的固定文件名：darwin、linux 用 .tar.gz，Windows 用 .zip。
func ArchiveName(version string, target Target) string {
	extension := ".tar.gz"
	if target.GOOS == "windows" {
		extension = ".zip"
	}
	return "pixiv-cli_" + version + "_" + target.GOOS + "_" + target.GOARCH + extension
}

// NotesFromChangelog 取出 changelog 中该版本唯一 H1 之后的正文；标题缺失或重复都返回错误。
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

// BilingualBody 拼出 GitHub Release 正文，英语与简体中文的标题和分隔符保持固定。
func BilingualBody(english, chinese []byte) []byte {
	return []byte("# English\n\n" + strings.TrimSpace(string(english)) + "\n\n---\n\n# 简体中文\n\n" + strings.TrimSpace(string(chinese)) + "\n")
}
