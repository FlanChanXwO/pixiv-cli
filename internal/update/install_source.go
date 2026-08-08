// Package update contains installation-source detection and update domain logic.
package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/build"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
)

const pixivCLIImportPath = "github.com/FlanChanXwO/pixiv-cli"

// InstallSource 表示当前二进制可由哪个安装渠道更新。
type InstallSource string

const (
	InstallSourceDevelopment    InstallSource = "development"
	InstallSourceHomebrewStable InstallSource = "homebrew-stable"
	InstallSourceHomebrewBeta   InstallSource = "homebrew-beta"
	InstallSourceGoInstall      InstallSource = "go-install"
	InstallSourceRelease        InstallSource = "release"
)

// DetectInstallSource 根据构建信息和当前可执行文件识别安装来源。
func DetectInstallSource(info buildinfo.Info) (InstallSource, error) {
	return detectInstallSource(info, defaultSourceDetector())
}

type sourceDetector struct {
	executable    func() (string, error)
	evalSymlinks  func(string) (string, error)
	readFile      func(string) ([]byte, error)
	readBuildInfo func() (*debug.BuildInfo, bool)
	getenv        func(string) string
	goos          string
}

func defaultSourceDetector() sourceDetector {
	return sourceDetector{
		executable:    os.Executable,
		evalSymlinks:  filepath.EvalSymlinks,
		readFile:      os.ReadFile,
		readBuildInfo: debug.ReadBuildInfo,
		getenv:        os.Getenv,
		goos:          runtime.GOOS,
	}
}

func detectInstallSource(info buildinfo.Info, deps sourceDetector) (InstallSource, error) {
	// 开发构建没有可更新的发布渠道，并且不读取环境或文件系统以保持判定无副作用。
	if info.IsDevelopment() {
		return InstallSourceDevelopment, nil
	}

	rawExecutable, err := deps.executable()
	if err != nil {
		return "", fmt.Errorf("determine executable path: %w", err)
	}
	actualExecutable, err := deps.evalSymlinks(rawExecutable)
	if err != nil {
		return "", fmt.Errorf("resolve executable symlink %q: %w", rawExecutable, err)
	}

	if source, isHomebrewKeg, err := detectHomebrewSource(actualExecutable, deps); isHomebrewKeg {
		return source, err
	}
	isGoInstalled, err := isGoInstall(actualExecutable, deps)
	if err != nil {
		return "", err
	}
	if isGoInstalled {
		return InstallSourceGoInstall, nil
	}
	return InstallSourceRelease, nil
}

func detectHomebrewSource(actualExecutable string, deps sourceDetector) (InstallSource, bool, error) {
	formula, kegRoot, ok := homebrewKeg(actualExecutable, pixivExecutableName(deps.goos))
	if !ok {
		return "", false, nil
	}

	receiptPath := filepath.Join(kegRoot, "INSTALL_RECEIPT.json")
	receipt, err := deps.readFile(receiptPath)
	if err != nil {
		return "", true, fmt.Errorf("read Homebrew receipt %q for executable %q: %w", receiptPath, actualExecutable, err)
	}
	var parsedReceipt struct {
		Source struct {
			Path string `json:"path"`
		} `json:"source"`
	}
	if err := json.Unmarshal(receipt, &parsedReceipt); err != nil {
		return "", true, fmt.Errorf("parse Homebrew receipt %q for executable %q: %w", receiptPath, actualExecutable, err)
	}
	receiptFormula, err := formulaFromReceiptPath(parsedReceipt.Source.Path)
	if err != nil {
		return "", true, fmt.Errorf("validate Homebrew receipt %q for keg %q: %w", receiptPath, kegRoot, err)
	}
	if receiptFormula != formula {
		return "", true, fmt.Errorf("Homebrew receipt %q formula %q does not match keg formula %q", receiptPath, receiptFormula, formula)
	}

	switch formula {
	case "pixiv-cli":
		return InstallSourceHomebrewStable, true, nil
	case "pixiv-cli-beta":
		return InstallSourceHomebrewBeta, true, nil
	default:
		return "", true, fmt.Errorf("unsupported Homebrew formula %q in keg %q for executable %q", formula, kegRoot, actualExecutable)
	}
}

func homebrewKeg(executablePath, executableName string) (formula, kegRoot string, ok bool) {
	executablePath = filepath.Clean(executablePath)
	if filepath.Base(executablePath) != executableName {
		return "", "", false
	}
	binDir := filepath.Dir(executablePath)
	if filepath.Base(binDir) != "bin" {
		return "", "", false
	}
	kegRoot = filepath.Dir(binDir)
	formulaDir := filepath.Dir(kegRoot)
	if filepath.Base(filepath.Dir(formulaDir)) != "Cellar" {
		return "", "", false
	}
	formula = filepath.Base(formulaDir)
	if formula == "." || formula == string(filepath.Separator) {
		return "", "", false
	}
	return formula, kegRoot, true
}

func isGoInstall(actualExecutable string, deps sourceDetector) (bool, error) {
	buildInfo, ok := deps.readBuildInfo()
	if !ok || buildInfo == nil || buildInfo.Main.Path != pixivCLIImportPath {
		return false, nil
	}

	binDir := deps.getenv("GOBIN")
	if binDir == "" {
		gopath := deps.getenv("GOPATH")
		if gopath == "" {
			// build.Default.GOPATH 已复用 Go 对 HOME/go 与 GOROOT 冲突的官方处理。
			gopath = build.Default.GOPATH
		}
		gopaths := filepath.SplitList(gopath)
		if len(gopaths) == 0 || gopaths[0] == "" {
			return false, nil
		}
		binDir = filepath.Join(gopaths[0], "bin")
	}
	expectedExecutable := filepath.Join(binDir, pixivExecutableName(deps.goos))
	resolvedExpected, err := deps.evalSymlinks(expectedExecutable)
	if err != nil {
		// 发布包也带有 Pixiv 的 Go 构建信息；预期的 go install 路径不存在时，
		// 仅说明当前程序并非由 go install 安装，不能阻断 release 更新检查。
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("resolve Go install executable symlink %q: %w", expectedExecutable, err)
	}
	return sameExecutablePath(actualExecutable, resolvedExpected, deps.goos), nil
}

func formulaFromReceiptPath(sourcePath string) (string, error) {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return "", fmt.Errorf("missing source.path")
	}
	formulaFile := path.Base(sourcePath)
	if path.Ext(formulaFile) != ".rb" {
		return "", fmt.Errorf("source.path %q does not name a formula file", sourcePath)
	}
	formula := strings.TrimSuffix(formulaFile, ".rb")
	if formula == "" {
		return "", fmt.Errorf("source.path %q has an empty formula name", sourcePath)
	}
	return formula, nil
}

func pixivExecutableName(goos string) string {
	if goos == "windows" {
		return "pixiv.exe"
	}
	return "pixiv"
}

func sameExecutablePath(first, second, goos string) bool {
	first = filepath.Clean(first)
	second = filepath.Clean(second)
	if goos == "windows" {
		return strings.EqualFold(first, second)
	}
	return first == second
}
