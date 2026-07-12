package update

import (
	"errors"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
)

const pixivCLIModulePath = "github.com/FlanChanXwO/pixiv-cli"

func TestDetectInstallSourceDevelopmentAvoidsSystemAccess(t *testing.T) {
	deps := sourceDetector{
		executable: func() (string, error) {
			t.Fatal("development build must not inspect the executable path")
			return "", nil
		},
		evalSymlinks: func(string) (string, error) {
			t.Fatal("development build must not resolve symlinks")
			return "", nil
		},
		readFile: func(string) ([]byte, error) {
			t.Fatal("development build must not read the filesystem")
			return nil, nil
		},
		readBuildInfo: func() (*debug.BuildInfo, bool) {
			t.Fatal("development build must not read Go build info")
			return nil, false
		},
	}

	got, err := detectInstallSource(buildinfo.Info{Version: "dev"}, deps)
	if err != nil {
		t.Fatalf("detectInstallSource() error = %v", err)
	}
	if got != InstallSourceDevelopment {
		t.Fatalf("detectInstallSource() = %q, want %q", got, InstallSourceDevelopment)
	}
}

func TestDetectInstallSourceHomebrewKeg(t *testing.T) {
	tests := []struct {
		name    string
		formula string
		want    InstallSource
	}{
		{name: "stable formula", formula: "pixiv-cli", want: InstallSourceHomebrewStable},
		{name: "beta formula", formula: "pixiv-cli-beta", want: InstallSourceHomebrewBeta},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rawPath, actualPath := homebrewExecutableFixture(t, test.formula, homebrewReceipt(test.formula))
			deps := testDetector(rawPath, actualPath, map[string]string{})
			deps.evalSymlinks = filepath.EvalSymlinks
			deps.readFile = os.ReadFile

			got, err := detectInstallSource(buildinfo.Info{Version: "v0.1.0"}, deps)
			if err != nil {
				t.Fatalf("detectInstallSource() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("detectInstallSource() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDetectInstallSourceRejectsInvalidHomebrewKeg(t *testing.T) {
	tests := []struct {
		name        string
		formula     string
		receipt     string
		removeFile  bool
		wantSubtext []string
	}{
		{
			name:        "malformed receipt",
			formula:     "pixiv-cli",
			receipt:     "{not-json",
			wantSubtext: []string{"INSTALL_RECEIPT.json", "parse Homebrew receipt"},
		},
		{
			name:        "missing receipt",
			formula:     "pixiv-cli",
			receipt:     homebrewReceipt("pixiv-cli"),
			removeFile:  true,
			wantSubtext: []string{"INSTALL_RECEIPT.json", "read Homebrew receipt"},
		},
		{
			name:        "unsupported formula",
			formula:     "other-pixiv",
			receipt:     homebrewReceipt("other-pixiv"),
			wantSubtext: []string{"other-pixiv", "unsupported Homebrew formula"},
		},
		{
			name:        "receipt formula does not match keg",
			formula:     "pixiv-cli",
			receipt:     homebrewReceipt("pixiv-cli-beta"),
			wantSubtext: []string{"INSTALL_RECEIPT.json", "does not match keg formula"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rawPath, actualPath := homebrewExecutableFixture(t, test.formula, test.receipt)
			receiptPath := filepath.Join(filepath.Dir(filepath.Dir(actualPath)), "INSTALL_RECEIPT.json")
			if test.removeFile {
				if err := os.Remove(receiptPath); err != nil {
					t.Fatalf("remove receipt fixture: %v", err)
				}
			}

			deps := testDetector(rawPath, actualPath, map[string]string{})
			deps.evalSymlinks = filepath.EvalSymlinks
			deps.readFile = os.ReadFile

			got, err := detectInstallSource(buildinfo.Info{Version: "v0.1.0"}, deps)
			if got != "" {
				t.Fatalf("detectInstallSource() source = %q, want no successful classification", got)
			}
			if err == nil {
				t.Fatal("detectInstallSource() error = nil, want visible Homebrew validation error")
			}
			for _, want := range test.wantSubtext {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("detectInstallSource() error = %q, want substring %q", err, want)
				}
			}
		})
	}
}

func TestDetectInstallSourceGoInstallWithExplicitGOBIN(t *testing.T) {
	gobin := filepath.Join(t.TempDir(), "bin")
	executable := filepath.Join(gobin, pixivExecutableName(runtime.GOOS))
	deps := testDetector(executable, executable, map[string]string{"GOBIN": gobin})
	deps.readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Path: pixivCLIModulePath + "/cmd/pixiv", Main: debug.Module{Path: pixivCLIModulePath}}, true
	}

	got, err := detectInstallSource(buildinfo.Info{Version: "v0.1.0"}, deps)
	if err != nil {
		t.Fatalf("detectInstallSource() error = %v", err)
	}
	if got != InstallSourceGoInstall {
		t.Fatalf("detectInstallSource() = %q, want %q", got, InstallSourceGoInstall)
	}
}

func TestDetectInstallSourceDoesNotMistakeGoBuildForGoInstall(t *testing.T) {
	tests := []struct {
		name          string
		executable    string
		buildInfo     *debug.BuildInfo
		buildInfoOkay bool
		env           map[string]string
	}{
		{
			name:          "matching module outside GOBIN",
			executable:    filepath.Join(t.TempDir(), "pixiv"),
			buildInfo:     &debug.BuildInfo{Path: pixivCLIModulePath + "/cmd/pixiv", Main: debug.Module{Path: pixivCLIModulePath}},
			buildInfoOkay: true,
			env:           map[string]string{"GOBIN": filepath.Join(t.TempDir(), "bin")},
		},
		{
			name:          "GOBIN executable with another main module",
			buildInfo:     &debug.BuildInfo{Path: pixivCLIModulePath, Main: debug.Module{Path: "example.com/not-pixiv"}},
			buildInfoOkay: true,
			env:           map[string]string{},
		},
		{
			name:          "GOBIN executable without build info",
			buildInfoOkay: false,
			env:           map[string]string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gobin := filepath.Join(t.TempDir(), "bin")
			if test.env == nil {
				test.env = map[string]string{}
			}
			if test.env["GOBIN"] == "" {
				test.env["GOBIN"] = gobin
			}
			if test.executable == "" {
				test.executable = filepath.Join(test.env["GOBIN"], pixivExecutableName(runtime.GOOS))
			}
			deps := testDetector(test.executable, test.executable, test.env)
			deps.readBuildInfo = func() (*debug.BuildInfo, bool) {
				return test.buildInfo, test.buildInfoOkay
			}

			got, err := detectInstallSource(buildinfo.Info{Version: "v0.1.0"}, deps)
			if err != nil {
				t.Fatalf("detectInstallSource() error = %v", err)
			}
			if got != InstallSourceRelease {
				t.Fatalf("detectInstallSource() = %q, want %q", got, InstallSourceRelease)
			}
		})
	}
}

func TestDetectInstallSourceUsesFirstGOPATHWhenGOBINIsUnset(t *testing.T) {
	firstGOPATH := t.TempDir()
	secondGOPATH := t.TempDir()
	executable := filepath.Join(firstGOPATH, "bin", pixivExecutableName(runtime.GOOS))
	deps := testDetector(executable, executable, map[string]string{"GOPATH": firstGOPATH + string(os.PathListSeparator) + secondGOPATH})
	deps.readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Path: pixivCLIModulePath + "/cmd/pixiv", Main: debug.Module{Path: pixivCLIModulePath}}, true
	}

	got, err := detectInstallSource(buildinfo.Info{Version: "v0.1.0"}, deps)
	if err != nil {
		t.Fatalf("detectInstallSource() error = %v", err)
	}
	if got != InstallSourceGoInstall {
		t.Fatalf("detectInstallSource() = %q, want %q", got, InstallSourceGoInstall)
	}
}

func TestDetectInstallSourceUsesDefaultGOPATHWhenEnvironmentIsUnset(t *testing.T) {
	executable := filepath.Join(build.Default.GOPATH, "bin", pixivExecutableName(runtime.GOOS))
	deps := testDetector(executable, executable, map[string]string{})
	deps.readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Path: pixivCLIModulePath + "/cmd/pixiv", Main: debug.Module{Path: pixivCLIModulePath}}, true
	}

	got, err := detectInstallSource(buildinfo.Info{Version: "v0.1.0"}, deps)
	if err != nil {
		t.Fatalf("detectInstallSource() error = %v", err)
	}
	if got != InstallSourceGoInstall {
		t.Fatalf("detectInstallSource() = %q, want %q", got, InstallSourceGoInstall)
	}
}

func TestDetectInstallSourceSurfacesBrokenExpectedGoInstallSymlink(t *testing.T) {
	root := t.TempDir()
	actualExecutable := filepath.Join(root, "release", pixivExecutableName(runtime.GOOS))
	if err := os.MkdirAll(filepath.Dir(actualExecutable), 0o755); err != nil {
		t.Fatalf("create release executable directory: %v", err)
	}
	if err := os.WriteFile(actualExecutable, []byte("fixture"), 0o755); err != nil {
		t.Fatalf("write release executable: %v", err)
	}
	gobin := filepath.Join(root, "bin")
	if err := os.MkdirAll(gobin, 0o755); err != nil {
		t.Fatalf("create GOBIN directory: %v", err)
	}
	expectedExecutable := filepath.Join(gobin, pixivExecutableName(runtime.GOOS))
	if err := os.Symlink(filepath.Join(root, "missing", pixivExecutableName(runtime.GOOS)), expectedExecutable); err != nil {
		t.Fatalf("create broken GOBIN symlink: %v", err)
	}

	deps := testDetector(actualExecutable, actualExecutable, map[string]string{"GOBIN": gobin})
	deps.evalSymlinks = filepath.EvalSymlinks
	deps.readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Path: pixivCLIModulePath + "/cmd/pixiv", Main: debug.Module{Path: pixivCLIModulePath}}, true
	}

	got, err := detectInstallSource(buildinfo.Info{Version: "v0.1.0"}, deps)
	if got != "" {
		t.Fatalf("detectInstallSource() source = %q, want no successful classification", got)
	}
	if err == nil {
		t.Fatal("detectInstallSource() error = nil, want expected Go install symlink resolution error")
	}
	// 错误上下文以 %q 呈现路径；Windows 路径中的反斜杠会随 Go 字符串规则转义。
	if !strings.Contains(err.Error(), fmt.Sprintf("%q", expectedExecutable)) || !strings.Contains(err.Error(), "resolve Go install executable symlink") {
		t.Fatalf("detectInstallSource() error = %q, want expected symlink path and context", err)
	}
}

func TestDetectInstallSourceTreatsUnknownFormalBinaryAsRelease(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "opt", pixivExecutableName(runtime.GOOS))
	deps := testDetector(executable, executable, map[string]string{})
	deps.readBuildInfo = func() (*debug.BuildInfo, bool) {
		return nil, false
	}

	got, err := detectInstallSource(buildinfo.Info{Version: "v0.1.0"}, deps)
	if err != nil {
		t.Fatalf("detectInstallSource() error = %v", err)
	}
	if got != InstallSourceRelease {
		t.Fatalf("detectInstallSource() = %q, want %q", got, InstallSourceRelease)
	}
}

func homebrewExecutableFixture(t *testing.T, formula, receipt string) (rawPath, actualPath string) {
	t.Helper()
	root := t.TempDir()
	actualPath = filepath.Join(root, "Cellar", formula, "0.1.0", "bin", pixivExecutableName(runtime.GOOS))
	if err := os.MkdirAll(filepath.Dir(actualPath), 0o755); err != nil {
		t.Fatalf("create keg fixture: %v", err)
	}
	if err := os.WriteFile(actualPath, []byte("fixture"), 0o755); err != nil {
		t.Fatalf("write keg executable fixture: %v", err)
	}
	receiptPath := filepath.Join(filepath.Dir(filepath.Dir(actualPath)), "INSTALL_RECEIPT.json")
	if err := os.WriteFile(receiptPath, []byte(receipt), 0o644); err != nil {
		t.Fatalf("write receipt fixture: %v", err)
	}
	rawPath = filepath.Join(root, "bin", pixivExecutableName(runtime.GOOS))
	if err := os.MkdirAll(filepath.Dir(rawPath), 0o755); err != nil {
		t.Fatalf("create raw executable directory: %v", err)
	}
	if err := os.Symlink(actualPath, rawPath); err != nil {
		t.Fatalf("create Homebrew executable symlink: %v", err)
	}
	return rawPath, actualPath
}

func homebrewReceipt(formula string) string {
	return `{"source":{"path":"Formula/` + formula + `.rb"}}`
}

func testDetector(executable, resolvedPath string, env map[string]string) sourceDetector {
	return sourceDetector{
		executable: func() (string, error) { return executable, nil },
		evalSymlinks: func(path string) (string, error) {
			if path == executable {
				return resolvedPath, nil
			}
			return path, nil
		},
		readFile: func(string) ([]byte, error) {
			return nil, errors.New("unexpected Homebrew receipt read")
		},
		readBuildInfo: func() (*debug.BuildInfo, bool) { return nil, false },
		getenv:        func(key string) string { return env[key] },
		goos:          runtime.GOOS,
	}
}
