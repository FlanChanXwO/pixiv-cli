//go:build windows

package installers_test

import (
	"archive/zip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallCmdInstallsVerifiedLatestArchive(t *testing.T) {
	fixtureDir, assetName := prepareWindowsFixture(t, false)
	fakeBin := prepareFakeCurlExe(t)
	installDir := filepath.Join(t.TempDir(), "install dir with spaces")

	output, err := runInstallCmd(t, fakeBin, fixtureDir, installDir)
	if err != nil {
		t.Fatalf("install verified fixture %s: %v\n%s", assetName, err, output)
	}
	installed := filepath.Join(installDir, "pixiv.exe")
	result, err := exec.Command(installed, "version", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("run installed fixture: %v\n%s", err, result)
	}
	if !strings.Contains(string(result), `"version":"v`+fixtureVersion+`"`) {
		t.Fatalf("installed fixture returned unexpected version: %s", result)
	}
	if !strings.Contains(string(output), "SHA-256 verified") || !strings.Contains(string(output), installed) {
		t.Fatalf("installer did not report verification and destination:\n%s", output)
	}
}

func TestInstallCmdChecksumFailurePreservesExistingBinary(t *testing.T) {
	fixtureDir, _ := prepareWindowsFixture(t, true)
	fakeBin := prepareFakeCurlExe(t)
	installDir := t.TempDir()
	installed := filepath.Join(installDir, "pixiv.exe")
	const sentinel = "existing-install-must-survive\r\n"
	if err := os.WriteFile(installed, []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}

	if output, err := runInstallCmd(t, fakeBin, fixtureDir, installDir); err == nil {
		t.Fatalf("checksum mismatch unexpectedly succeeded:\n%s", output)
	}
	payload, err := os.ReadFile(installed)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != sentinel {
		t.Fatalf("checksum failure changed existing binary: %q", payload)
	}
}

func runInstallCmd(t *testing.T, fakeBin, fixtureDir, installDir string) ([]byte, error) {
	t.Helper()
	script, err := filepath.Abs(filepath.Join("..", "install.cmd"))
	if err != nil {
		t.Fatal(err)
	}
	tempRoot := t.TempDir()
	command := exec.Command("cmd.exe", installCmdInvocation(script, installDir)...)
	command.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PIXIV_INSTALLER_FIXTURES="+fixtureDir,
		"TEMP="+tempRoot,
		"TMP="+tempRoot,
	)
	output, runErr := command.CombinedOutput()
	requireNoWindowsInstallerTemporaryDirectory(t, tempRoot)
	return output, runErr
}

func requireNoWindowsInstallerTemporaryDirectory(t *testing.T, tempRoot string) {
	t.Helper()
	entries, err := os.ReadDir(tempRoot)
	if err != nil {
		t.Fatalf("read isolated installer TEMP: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(strings.ToLower(entry.Name()), "pixiv-install-") {
			t.Fatalf("Windows installer left temporary directory %q", entry.Name())
		}
	}
}

func prepareWindowsFixture(t *testing.T, corruptChecksum bool) (string, string) {
	t.Helper()
	targetArch := map[string]string{"amd64": "amd64", "arm64": "arm64"}[runtime.GOARCH]
	if targetArch == "" {
		t.Fatalf("unsupported Windows test architecture %q", runtime.GOARCH)
	}
	directory := t.TempDir()
	pixivExe := filepath.Join(directory, "pixiv.exe")
	buildWindowsHelper(t, pixivExe, `package main
import (
  "fmt"
  "os"
)
func main() {
  if len(os.Args) < 2 || os.Args[1] != "version" { os.Exit(64) }
  fmt.Println("{\"version\":\"v`+fixtureVersion+`\"}")
}
`)

	assetName := fmt.Sprintf("pixiv-cli_%s_windows_%s.zip", fixtureVersion, targetArch)
	assetPath := filepath.Join(directory, assetName)
	writeZip(t, assetPath, "pixiv.exe", pixivExe)
	payload, err := os.ReadFile(assetPath)
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(payload))
	if corruptChecksum {
		digest = strings.Repeat("0", sha256.Size*2)
	}
	checksums := digest + "  " + assetName + "\r\n" +
		strings.Repeat("1", sha256.Size*2) + "  install.cmd\r\n" +
		strings.Repeat("2", sha256.Size*2) + "  install.sh\r\n"
	if err := os.WriteFile(filepath.Join(directory, "checksums.txt"), []byte(checksums), 0o600); err != nil {
		t.Fatal(err)
	}
	return directory, assetName
}

func prepareFakeCurlExe(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	path := filepath.Join(directory, "curl.exe")
	buildWindowsHelper(t, path, `package main
import (
  "fmt"
  "io"
  "os"
  "path/filepath"
  "strings"
)
func main() {
  var output, url string
  for index := 1; index < len(os.Args); index++ {
    if os.Args[index] == "-o" || os.Args[index] == "--output" {
      index++
      if index >= len(os.Args) { os.Exit(64) }
      output = os.Args[index]
      continue
    }
    if !strings.HasPrefix(os.Args[index], "-") { url = os.Args[index] }
  }
  if output == "" || url == "" { os.Exit(64) }
  if slash := strings.LastIndexAny(url, "/\\"); slash >= 0 { url = url[slash+1:] }
  source, err := os.Open(filepath.Join(os.Getenv("PIXIV_INSTALLER_FIXTURES"), url))
  if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
  defer source.Close()
  destination, err := os.Create(output)
  if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
  if _, err = io.Copy(destination, source); err != nil { destination.Close(); fmt.Fprintln(os.Stderr, err); os.Exit(1) }
  if err = destination.Close(); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
}
`)
	return directory
}

func buildWindowsHelper(t *testing.T, output, source string) {
	t.Helper()
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "main.go")
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "build", "-trimpath", "-o", output, sourcePath)
	command.Env = append(os.Environ(), "CGO_ENABLED=0")
	if result, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build Windows fixture helper: %v\n%s", err, result)
	}
}

func writeZip(t *testing.T, path, member, sourcePath string) {
	t.Helper()
	archive, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(archive)
	entry, err := writer.Create(member)
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(entry, source); err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
}
