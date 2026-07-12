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

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
)

func TestRustUgoiraEncoderNativeGIFAndAPNG(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "ugoira.zip")
	createZip(t, zipPath, "000000.jpg", rustUgoiraJPEG(t))
	frames := []pixiv.UgoiraFrame{{File: "000000.jpg", Delay: 80}}

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
	command := exec.Command("go", "build", "./cmd/pixiv")
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
		line := string(body)
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

// TestPinnedRustSourcesDisableGitTextConversion 保留 Cargo vendor 和本地 locked dependency 的精确字节；
// Windows checkout 若把 LF 转为 CRLF，会破坏 Cargo checksum、licensebundle 或 staticlib source digest。
func TestPinnedRustSourcesDisableGitTextConversion(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, source := range []string{
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
			t.Fatalf("Cargo vendor source %q must disable Git text conversion, got %q", source, strings.TrimSpace(string(body)))
		}
	}
}

// TestGeneratedRustLicenseOutputsUseLFCheckout 锁定 licensebundle 的两个受检生成物在 Windows
// checkout 也保留 LF；生成器规范化为 LF，若 Git 改写为 CRLF，release gate 会错误报告 stale。
func TestGeneratedRustLicenseOutputsUseLFCheckout(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, source := range []string{
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
			t.Fatalf("generated Rust license output %q must use LF checkout, got %q", source, strings.TrimSpace(output))
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
