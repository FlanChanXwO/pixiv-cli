// Package releasecontract 只拥有 release 与 native evidence 共用的发布 identity：
// Version/Channel、六个 archive 目标、确定性的 Go↔Rust target 映射与 archive name。
// runner、Rust toolchain 与 CC 等可变平台 metadata 只由 ci/platforms.json 持有。
package releasecontract

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/releaseversion"
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

// RustTarget 返回 target 对应的 Rust target triple。
func (t Target) RustTarget() string {
	arch := t.GOARCH
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "aarch64"
	}
	switch t.GOOS {
	case "darwin":
		return arch + "-apple-darwin"
	case "linux":
		return arch + "-unknown-linux-gnu"
	case "windows":
		return arch + "-pc-windows-msvc"
	default:
		return ""
	}
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
	return releaseversion.Validate(version)
}

// Channel 返回 release 的稳定渠道名称。调用方必须先 ValidateVersion；build metadata
// 内合法的连字符不会被视为 prerelease 分隔符。
func Channel(version string) string {
	return releaseversion.Channel(version)
}
