//go:build darwin

package loginhelper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	constants "github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
)

const pixivURLHandlerBundleID = "com.flanchan.pixiv-cli.url-handler"

// pixivURLHandlerSourceVersion 变更时强制重编译已安装的 helper，确保修复能覆盖旧 bundle。
const pixivURLHandlerSourceVersion = "4"

var (
	queryDarwinURLSchemeHandler = defaultURLSchemeHandler
	setDarwinURLSchemeHandler   = setDefaultURLSchemeHandler
)

// EnsurePersistent 注册按需启动的 macOS pixiv:// helper。manifest 只保存旧
// bundle identifier 与当前二进制路径，绝不保存 remote relay secret。
func EnsurePersistent(ctx context.Context) error {
	executablePath, err := os.Executable()
	if err != nil {
		return err
	}
	appPath, err := pixivURLHandlerAppPath()
	if err != nil {
		return err
	}
	if err := ensurePixivURLHandlerApp(ctx, appPath); err != nil {
		return err
	}
	manifest, exists, err := loadHandlerManifest()
	if err != nil {
		return err
	}
	if !exists {
		previous, err := queryDarwinURLSchemeHandler(ctx, "pixiv")
		if err != nil {
			return err
		}
		manifest = handlerManifest{Version: 1, ExecutablePath: executablePath, PreviousHandler: previous}
	} else {
		// upgrade 或二进制路径变化时只刷新启动目标，保留初次接管时的旧 handler。
		manifest.ExecutablePath = executablePath
	}
	if err := registerURLHandlerApp(ctx, appPath); err != nil {
		return err
	}
	if err := setDarwinURLSchemeHandler(ctx, "pixiv", pixivURLHandlerBundleID); err != nil {
		return err
	}
	if err := saveHandlerManifest(manifest); err != nil {
		// manifest 写入失败时尽力恢复，避免留下无法由 pixiv-cli 清理的默认关联。
		if manifest.PreviousHandler != "" && manifest.PreviousHandler != pixivURLHandlerBundleID {
			_ = setDarwinURLSchemeHandler(context.Background(), "pixiv", manifest.PreviousHandler)
		}
		return err
	}
	return nil
}

// DisablePersistent 仅当 pixiv-cli 仍是默认 handler 时才恢复用户之前的选择。
// 如果用户后来自行选择了其他应用，绝不能覆盖该新选择。
func DisablePersistent(ctx context.Context) error {
	manifest, exists, err := loadHandlerManifest()
	if err != nil || !exists {
		return err
	}
	current, err := queryDarwinURLSchemeHandler(ctx, "pixiv")
	if err != nil {
		return err
	}
	if current == pixivURLHandlerBundleID {
		if manifest.PreviousHandler == "" || manifest.PreviousHandler == pixivURLHandlerBundleID {
			// LaunchServices 没有稳定、受支持的“清空 scheme 默认 handler”操作。
			// 没有旧 handler 时宁可保留 manifest 让用户先在系统 UI 选择目标，也
			// 不能删掉恢复信息后把 pixiv-cli 静默留为默认应用。
			return errors.New("cannot safely restore a previous macOS Pixiv URL handler")
		}
		if err := setDarwinURLSchemeHandler(ctx, "pixiv", manifest.PreviousHandler); err != nil {
			return err
		}
	}
	return removeHandlerManifest()
}

// DelegateToPrevious 精确定向启动此前记录的 macOS bundle。它不通过 shell
// 拼接 URL；无旧 handler 时向系统调用方返回真实错误。
func DelegateToPrevious(ctx context.Context, rawURL string) error {
	manifest, exists, err := loadHandlerManifest()
	if err != nil {
		return err
	}
	if !exists || manifest.PreviousHandler == "" || manifest.PreviousHandler == pixivURLHandlerBundleID {
		return errors.New("no previous Pixiv URL handler is available")
	}
	if err := exec.CommandContext(ctx, "open", "-b", manifest.PreviousHandler, rawURL).Run(); err != nil {
		return errors.New("could not open previous Pixiv URL handler")
	}
	return nil
}

// Install 为本次登录安装 pixiv:// 回调 helper，并返回清理函数。
func Install(ctx context.Context, callbackRelayURL string) (func(), error) {
	if _, err := exec.LookPath("swiftc"); err != nil {
		return nil, err
	}
	if _, err := exec.LookPath("swift"); err != nil {
		return nil, err
	}
	appPath, err := pixivURLHandlerAppPath()
	if err != nil {
		return nil, err
	}
	if err := ensurePixivURLHandlerApp(ctx, appPath); err != nil {
		return nil, err
	}
	endpointPath, err := writeCallbackEndpoint(callbackRelayURL)
	if err != nil {
		return nil, err
	}
	installed := false
	defer func() {
		if !installed {
			_ = os.Remove(endpointPath)
		}
	}()
	previous, _ := queryDarwinURLSchemeHandler(ctx, "pixiv")
	if err := registerURLHandlerApp(ctx, appPath); err != nil {
		return nil, err
	}
	if err := setDarwinURLSchemeHandler(ctx, "pixiv", pixivURLHandlerBundleID); err != nil {
		if previous != "" && previous != pixivURLHandlerBundleID {
			_ = setDarwinURLSchemeHandler(context.Background(), "pixiv", previous)
		}
		return nil, err
	}
	installed = true
	cleanup := func() {
		_ = os.Remove(endpointPath)
		if previous != "" && previous != pixivURLHandlerBundleID {
			_ = setDarwinURLSchemeHandler(context.Background(), "pixiv", previous)
		}
	}
	return cleanup, nil
}

func pixivURLHandlerAppPath() (string, error) {
	appDataDir, err := files.UserDataSubdir(constants.AppDataDirName)
	if err != nil {
		return "", err
	}
	return filepath.Join(appDataDir, "url-handler", "PixivCLIURLHandler.app"), nil
}

func ensurePixivURLHandlerApp(ctx context.Context, appPath string) error {
	return ensurePixivURLHandlerAppWithCompiler(ctx, appPath, compilePixivURLHandler)
}

type pixivURLHandlerCompiler func(context.Context, string, string) ([]byte, error)

func ensurePixivURLHandlerAppWithCompiler(ctx context.Context, appPath string, compile pixivURLHandlerCompiler) error {
	executablePath := filepath.Join(appPath, "Contents", "MacOS", "PixivCLIURLHandler")
	infoPath := filepath.Join(appPath, "Contents", "Info.plist")
	versionPath := filepath.Join(appPath, "Contents", "Resources", "source-version")
	if fileExists(executablePath) && fileExists(infoPath) && handlerSourceVersionMatches(versionPath) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(executablePath), constants.PrivateDirMode); err != nil {
		return err
	}
	// Swift 源码可能包含后续敏感回调逻辑；私有随机目录和独占创建共同避免固定路径的 symlink 覆盖及并发竞态。
	sourceDir, err := os.MkdirTemp("", "pixiv-cli-url-handler-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(sourceDir)
	if err := os.Chmod(sourceDir, constants.PrivateDirMode); err != nil {
		return err
	}
	source, err := os.CreateTemp(sourceDir, "url-handler-*.swift")
	if err != nil {
		return err
	}
	sourcePath := source.Name()
	if err := source.Chmod(constants.PrivateFileMode); err != nil {
		_ = source.Close()
		return err
	}
	if _, err := source.WriteString(pixivURLHandlerSwiftSource); err != nil {
		_ = source.Close()
		return err
	}
	if err := source.Close(); err != nil {
		return err
	}
	if out, err := compile(ctx, sourcePath, executablePath); err != nil {
		return fmt.Errorf("compile pixiv:// callback helper: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if err := os.WriteFile(infoPath, []byte(pixivURLHandlerInfoPlist), constants.PrivateFileMode); err != nil {
		return err
	}
	if err := files.WritePrivateFile(versionPath, []byte(pixivURLHandlerSourceVersion+"\n"), constants.PrivateFileMode); err != nil {
		return err
	}
	return nil
}

func handlerSourceVersionMatches(versionPath string) bool {
	version, err := os.ReadFile(versionPath)
	return err == nil && strings.TrimSpace(string(version)) == pixivURLHandlerSourceVersion
}

func compilePixivURLHandler(ctx context.Context, sourcePath, executablePath string) ([]byte, error) {
	return exec.CommandContext(ctx, "swiftc", sourcePath, "-o", executablePath).CombinedOutput()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func registerURLHandlerApp(ctx context.Context, appPath string) error {
	lsregister := "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
	cmd := exec.CommandContext(ctx, lsregister, "-f", appPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("register pixiv:// callback helper: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func defaultURLSchemeHandler(ctx context.Context, scheme string) (string, error) {
	script := fmt.Sprintf(`import Foundation; import CoreServices; let scheme=%q as NSString; if let h = LSCopyDefaultHandlerForURLScheme(scheme)?.takeRetainedValue() { print(h as String) }`, scheme)
	out, err := exec.CommandContext(ctx, "swift", "-e", script).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func setDefaultURLSchemeHandler(ctx context.Context, scheme, bundleID string) error {
	script := fmt.Sprintf(`import Foundation; import CoreServices; let scheme=%q as NSString; let handler=%q as NSString; let status = LSSetDefaultHandlerForURLScheme(scheme, handler); if status != 0 { exit(1) }`, scheme, bundleID)
	if out, err := exec.CommandContext(ctx, "swift", "-e", script).CombinedOutput(); err != nil {
		return fmt.Errorf("set %s:// callback handler: %w: %s", scheme, err, strings.TrimSpace(string(out)))
	}
	return nil
}

const pixivURLHandlerInfoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key><string>PixivCLIURLHandler</string>
  <key>CFBundleIdentifier</key><string>` + pixivURLHandlerBundleID + `</string>
  <key>CFBundleName</key><string>PixivCLIURLHandler</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSUIElement</key><true/>
  <key>CFBundleURLTypes</key>
  <array>
    <dict>
      <key>CFBundleURLName</key><string>Pixiv CLI OAuth Callback</string>
      <key>CFBundleURLSchemes</key>
      <array><string>pixiv</string></array>
    </dict>
  </array>
</dict>
</plist>
`

const pixivURLHandlerSwiftSource = `import Cocoa
import Foundation

final class AppDelegate: NSObject, NSApplicationDelegate {
    override init() {
        super.init()
        NSAppleEventManager.shared().setEventHandler(self, andSelector: #selector(handleGetURLEvent(_:withReplyEvent:)), forEventClass: AEEventClass(kInternetEventClass), andEventID: AEEventID(kAEGetURL))
    }

	@objc func handleGetURLEvent(_ event: NSAppleEventDescriptor, withReplyEvent replyEvent: NSAppleEventDescriptor) {
		guard let callbackURL = event.paramDescriptor(forKeyword: keyDirectObject)?.stringValue else {
			NSApp.terminate(nil)
			return
		}
		runCallbackHandler(callbackURL)
		NSApp.terminate(nil)
	}

	private func runCallbackHandler(_ callbackURL: String) {
		guard let manifestURL = manifestURL(),
			  let data = try? Data(contentsOf: manifestURL),
			  let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
			  let executable = object["executable_path"] as? String,
			  !executable.isEmpty else {
			return
		}
		let process = Process()
		process.executableURL = URL(fileURLWithPath: executable)
		process.arguments = ["auth", "_callback", callbackURL]
		try? process.run()
	}

    private func manifestURL() -> URL? {
        // manifest 与 Go 端 handlerManifestPath 一致；它没有 bearer secret，只有
        // 需被按需启动的当前 pixiv-cli binary 路径。
        return URL(fileURLWithPath: NSHomeDirectory())
            .appendingPathComponent(".pixiv-cli/url-handler/handler-manifest.json")
    }
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.setActivationPolicy(.accessory)
app.run()
`
