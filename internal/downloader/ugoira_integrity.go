package downloader

import (
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/downloader/staticlib"
)

// UgoiraStaticlibManifest 保持下载包既有的清单 API；实现位于无 cgo 依赖的 staticlib 子包，
// 使 native evidence 的 policy gate 能在目标库生成前运行。
type UgoiraStaticlibManifest = staticlib.Manifest

// UgoiraStaticlibManifestAsset 保持下载包既有的 artifact API。
type UgoiraStaticlibManifestAsset = staticlib.ManifestAsset

var ugoiraStaticlibPlatformTargets = map[string]string{
	"darwin/amd64":  "x86_64-apple-darwin",
	"darwin/arm64":  "aarch64-apple-darwin",
	"linux/amd64":   "x86_64-unknown-linux-gnu",
	"linux/arm64":   "aarch64-unknown-linux-gnu",
	"windows/amd64": "x86_64-pc-windows-msvc",
	"windows/arm64": "aarch64-pc-windows-msvc",
}

// expectedStaticlibManifestPath 保持下载包既有测试与固定平台布局的辅助约束。
func expectedStaticlibManifestPath(platform, target string) string {
	archive := "libugoira_rs.a"
	if strings.HasPrefix(platform, "windows/") {
		archive = "ugoira_rs.lib"
	}
	return target + "/" + archive
}

// CalculateUgoiraRustSourceDigest 保持下载路径对 Rust source identity 的既有入口。
func CalculateUgoiraRustSourceDigest(crateDir, quantetteDir string) (string, error) {
	return staticlib.CalculateRustSourceDigest(crateDir, quantetteDir)
}

// ValidateUgoiraStaticlibManifest 保持下载路径对 manifest 格式校验的既有入口。
func ValidateUgoiraStaticlibManifest(body []byte, sourceDigest string) error {
	return staticlib.ValidateManifest(body, sourceDigest)
}

// ValidateUgoiraStaticlibManifestFiles 保持下载路径对 manifest 与真实 staticlib 校验的既有入口。
func ValidateUgoiraStaticlibManifestFiles(staticlibDir string, body []byte, sourceDigest string) error {
	return staticlib.ValidateManifestFiles(staticlibDir, body, sourceDigest)
}
