//go:build cgo && ((darwin && (amd64 || arm64)) || (linux && (amd64 || arm64)) || (windows && (amd64 || arm64)))

package download

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestRustUgoiraEncoderNativeGIFAndAPNG(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "ugoira.zip")
	createZip(t, zipPath, "000000.jpg", rustUgoiraJPEG(t))
	frames := []pixiv.UgoiraFrame{{Filename: "000000.jpg", DelayMilliseconds: 80}}

	gifPath := filepath.Join(dir, "out.gif")
	if err := NewRustUgoiraEncoder().Encode(context.Background(), UgoiraEncodeInput{
		ZipPath: zipPath, Frames: frames, WorkDir: dir, OutputPath: gifPath, Format: AnimationFormatGIF,
	}); err != nil {
		t.Fatalf("encode GIF: %v", err)
	}
	gifFile, err := os.Open(gifPath)
	if err != nil {
		t.Fatal(err)
	}
	defer gifFile.Close()
	if _, err := gif.Decode(gifFile); err != nil {
		t.Fatalf("decode GIF: %v", err)
	}

	apngPath := filepath.Join(dir, "out.apng")
	if err := NewRustUgoiraEncoder().Encode(context.Background(), UgoiraEncodeInput{
		ZipPath: zipPath, Frames: frames, WorkDir: dir, OutputPath: apngPath, Format: AnimationFormatAPNG,
	}); err != nil {
		t.Fatalf("encode APNG: %v", err)
	}
	body, err := os.ReadFile(apngPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(body, []byte("\x89PNG\r\n\x1a\n")) || !bytes.Contains(body, []byte("acTL")) {
		t.Fatalf("APNG output does not contain a PNG signature and animation control chunk")
	}
	apngFile, err := os.Open(apngPath)
	if err != nil {
		t.Fatal(err)
	}
	defer apngFile.Close()
	if _, err := png.Decode(apngFile); err != nil {
		t.Fatalf("decode APNG base PNG frame: %v", err)
	}
}

func TestCGODisabledBuildRejectsMissingRustStaticlib(t *testing.T) {
	// 顶层 pixiv 是公开 SDK 源码目录；显式输出到临时目录，确保本测试验证的是
	// CGO/staticlib 编译门，而不是 Go 对同名默认二进制输出的目录冲突。
	command := exec.Command("go", "build", "-o", filepath.Join(t.TempDir(), "pixiv"), "./cmd/pixiv")
	command.Dir = filepath.Clean(filepath.Join("..", ".."))
	command.Env = append(os.Environ(), "CGO_ENABLED=0")
	body, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("CGO_ENABLED=0 go build unexpectedly succeeded")
	}
	message := strings.ToLower(string(body))
	for _, want := range []string{"go 1.26.3", "cgo", "staticlib", "c linker"} {
		if !strings.Contains(message, want) {
			t.Fatalf("CGO_ENABLED=0 build error does not contain %q:\n%s", want, body)
		}
	}
}

// TestLinuxRustStaticlibLinkersLinkSystemMath 锁住 GitHub Linux runner 的真实链接需求：
// image/Rust staticlib 会引用 libm 的 sinf/expf，不能仅把 archive 传给 cgo linker。
func TestLinuxRustStaticlibLinkersLinkSystemMath(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, source := range []string{
		"internal/download/ugoira_rust_link_linux_amd64.go",
		"internal/download/ugoira_rust_link_linux_arm64.go",
	} {
		body, err := os.ReadFile(filepath.Join(repoRoot, source))
		if err != nil {
			t.Fatalf("read Linux cgo selector %q: %v", source, err)
		}
		if !strings.Contains(string(body), "#cgo LDFLAGS:") || !strings.Contains(string(body), " -lm") {
			t.Fatalf("Linux cgo selector %q must link Rust staticlib with libm (-lm):\n%s", source, body)
		}
	}
}

// TestWindowsRustStaticlibSelectorsUseCgoLibrarySearchFlags 锁住 Windows 的 cgo 参数形态：
// `${SRCDIR}` 会展开为含驱动器号的绝对路径，直接传 `.lib` 会被 Go 的 cgo 安全校验拒绝；
// 通过受支持的 -L/-l 传递目录与库名，才能让 Windows LLVM linker 找到 MSVC staticlib。
func TestWindowsRustStaticlibSelectorsUseCgoLibrarySearchFlags(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, source := range []string{
		"internal/download/ugoira_rust_link_windows_amd64.go",
		"internal/download/ugoira_rust_link_windows_arm64.go",
	} {
		body, err := os.ReadFile(filepath.Join(repoRoot, source))
		if err != nil {
			t.Fatalf("read Windows cgo selector %q: %v", source, err)
		}
		// Windows checkout 可能把普通 Go 源文件转换为 CRLF；cgo 注释位置的
		// 语义不随行尾表示变化，断言前统一为 LF 以免把注释误判进 preamble。
		line := strings.ReplaceAll(string(body), "\r\n", "\n")
		for _, want := range []string{
			"#cgo LDFLAGS: -L${SRCDIR}/ugoira_rs/staticlib/",
			" -lugoira_rs",
			" -ladvapi32",
			" -lntdll",
			" -luserenv",
			" -lws2_32",
			" -ldbghelp",
		} {
			if !strings.Contains(line, want) {
				t.Fatalf("Windows cgo selector %q must link Rust native dependency %q:\n%s", source, want, body)
			}
		}
		if strings.Contains(line, "/ugoira_rs.lib\n") {
			t.Fatalf("Windows cgo selector %q must not pass ugoira_rs.lib as a direct linker input:\n%s", source, body)
		}
		if strings.Contains(line, "package download\n\n// Rust staticlib") || !strings.Contains(line, "import \"C\"\n\n// Rust staticlib") {
			t.Fatalf("Windows cgo selector %q must keep its Chinese explanation outside the cgo C preamble:\n%s", source, body)
		}
	}
}

// TestWindowsNativeEvidenceUsesLLDBackedClang 锁住 Windows workflow 的外链驱动：
// Go 仅在外链器报告 LLD 时跳过 GCC 专属的 debug linker script；MSVC `link.exe` 不能解析该脚本。
func TestWindowsNativeEvidenceUsesLLDBackedClang(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	body, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", "native-evidence.yml"))
	if err != nil {
		t.Fatalf("read native evidence workflow: %v", err)
	}
	if strings.Count(string(body), "export CC='clang -fuse-ld=lld'") != 2 {
		t.Fatalf("Windows smoke and binary build must each select clang backed by lld:\n%s", body)
	}
}

// TestPinnedRustSourcesDisableGitTextConversion 保留 first-party crate、Cargo vendor 和本地
// locked dependency 的精确字节；Windows checkout 若把 LF 转为 CRLF，会改变
// staticlib source digest，或破坏 Cargo checksum 与 licensebundle 输入。
func TestPinnedRustSourcesDisableGitTextConversion(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, source := range []string{
		"internal/download/ugoira_rs/Cargo.toml",
		"internal/download/ugoira_rs/Cargo.lock",
		"internal/download/ugoira_rs/build.rs",
		"internal/download/ugoira_rs/.cargo/config.toml",
		"internal/download/ugoira_rs/src/lib.rs",
		"internal/download/ugoira_rs/src/nested/digest-input.rs",
		"internal/download/ugoira_rs/vendor/crc32fast/benches/bench.rs",
		"internal/download/ugoira_rs/vendor/crc32fast/src/specialized/mod.rs",
		"third_party/rust/quantette-0.6.0/LICENSE-APACHE",
	} {
		command := exec.Command("git", "-C", repoRoot, "check-attr", "text", "--", source)
		body, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("read Git text attribute for %q: %v\n%s", source, err, body)
		}
		if !strings.Contains(string(body), "text: unset") {
			t.Fatalf("Rust source identity input %q must disable Git text conversion, got %q", source, strings.TrimSpace(string(body)))
		}
	}

	nonDigestSource := "internal/download/ugoira_rs/src/not-a-digest.md"
	command := exec.Command("git", "-C", repoRoot, "check-attr", "text", "--", nonDigestSource)
	body, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("read Git text attribute for %q: %v\n%s", nonDigestSource, err, body)
	}
	if !strings.Contains(string(body), "text: unspecified") {
		t.Fatalf("non-digest Rust crate file %q must keep the default Git text behavior, got %q", nonDigestSource, strings.TrimSpace(string(body)))
	}
}

// TestReleaseLicenseOutputsUseLFCheckout 锁定发布归档的许可证输入在 Windows checkout 也保留 LF；
// 否则 runner record 虽自洽，跨平台 consolidation 仍会因 archive member bytes 不一致而拒绝证据。
func TestReleaseLicenseOutputsUseLFCheckout(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, source := range []string{
		"LICENSE",
		"THIRD_PARTY_LICENSES.md",
		"third_party/licenses/crc32fast-1.5.0/LICENSE-MIT",
	} {
		command := exec.Command("git", "-C", repoRoot, "check-attr", "text", "eol", "--", source)
		body, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("read Git line-ending attributes for %q: %v\n%s", source, err, body)
		}
		output := string(body)
		if !strings.Contains(output, "text: set") || !strings.Contains(output, "eol: lf") {
			t.Fatalf("release license input %q must use LF checkout, got %q", source, strings.TrimSpace(output))
		}
	}
}

func rustUgoiraJPEG(t *testing.T) []byte {
	t.Helper()
	var body bytes.Buffer
	imageBody := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			imageBody.SetRGBA(x, y, color.RGBA{R: 30, G: 160, B: 220, A: 255})
		}
	}
	body.Reset()
	if err := jpeg.Encode(&body, imageBody, nil); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}
