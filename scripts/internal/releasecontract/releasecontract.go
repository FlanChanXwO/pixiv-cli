// Package releasecontract 只拥有 release 与 native evidence verifier 共用的发布契约：
// Version/Channel、六平台、Go↔Rust 映射与工具链 pins、archive identity/name。
package releasecontract

import (
	"fmt"
	"regexp"
	"strings"
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

var semanticVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

// ValidateVersion 要求 version 是不带前导 v 的 semantic version。
func ValidateVersion(version string) error {
	if !semanticVersionPattern.MatchString(version) {
		return fmt.Errorf("version must be a semantic version without a leading v: %q", version)
	}
	return nil
}

// Channel 返回 release 的稳定渠道名称。调用方必须先 ValidateVersion；build metadata
// 内合法的连字符不会被视为 prerelease 分隔符。
func Channel(version string) string {
	coreVersion, _, _ := strings.Cut(version, "+")
	if strings.Contains(coreVersion, "-") {
		return "prerelease"
	}
	return "stable"
}

var pinnedRustToolchains = map[string]string{
	"x86_64-apple-darwin":       "1.96.0",
	"aarch64-apple-darwin":      "1.96.1",
	"x86_64-unknown-linux-gnu":  "1.96.1",
	"aarch64-unknown-linux-gnu": "1.96.1",
	"x86_64-pc-windows-msvc":    "1.96.0",
	"aarch64-pc-windows-msvc":   "1.96.1",
}

// PinnedRustToolchain 返回 release 与 native evidence 共同审计的目标工具链。
// staticlib 的字节身份包含 rustc，因此未知目标必须 fail closed。
func PinnedRustToolchain(target string) (string, bool) {
	toolchain, ok := pinnedRustToolchains[target]
	return toolchain, ok
}
