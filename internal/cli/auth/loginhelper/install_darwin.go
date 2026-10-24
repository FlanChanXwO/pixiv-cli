//go:build darwin

// macOS pixiv:// helper installation, LaunchServices registration, and
// delegation to the previously registered bundle.

package loginhelper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	filesecret "github.com/FlanChanXwO/pixiv-cli/internal/storage/secretfile"
)

// pixivURLHandlerBundleID 是 pixiv:// helper 在 macOS 上注册的 bundle identifier。
const pixivURLHandlerBundleID = "com.flanchan.pixiv-cli.url-handler"

// pixivURLHandlerSourceVersion 变更时强制重编译已安装的 helper，确保修复能覆盖旧 bundle。
const pixivURLHandlerSourceVersion = "7"

// urlHandlerInstaller owns the macOS LaunchServices and helper-compilation
// boundaries. Production code uses newURLHandlerInstaller; tests construct it with
// recording implementations.
type urlHandlerInstaller struct {
	executable   func() (string, error)
	homeDir      func() (string, error)
	lookPath     func(string) (string, error)
	queryScheme  func(context.Context, string) (string, error)
	setScheme    func(context.Context, string, string) error
	compile      func(context.Context, string, string) ([]byte, error)
	registerApp  func(context.Context, string) error
	openPrevious func(context.Context, string, string) error
}

func newURLHandlerInstaller() urlHandlerInstaller {
	return urlHandlerInstaller{
		executable:  os.Executable,
		homeDir:     os.UserHomeDir,
		lookPath:    exec.LookPath,
		queryScheme: defaultURLSchemeHandler,
		setScheme:   setDefaultURLSchemeHandler,
		compile:     compilePixivURLHandler,
		registerApp: registerURLHandlerApp,
		openPrevious: func(ctx context.Context, bundleID, rawURL string) error {
			return exec.CommandContext(ctx, "open", "-b", bundleID, rawURL).Run()
		},
	}
}

// ensurePersistent 注册按需启动的 macOS pixiv:// helper。manifest 只保存旧
// bundle identifier 与当前二进制路径，绝不保存 remote relay secret。
func (i urlHandlerInstaller) ensurePersistent(ctx context.Context) error {
	executablePath, err := i.executable()
	if err != nil {
		return err
	}
	homeDirectory, err := i.homeDir()
	if err != nil {
		return err
	}
	appPath, err := pixivURLHandlerAppPath()
	if err != nil {
		return err
	}
	if err := i.ensureApp(ctx, appPath); err != nil {
		return err
	}
	manifest, exists, err := loadHandlerManifest()
	if err != nil {
		return err
	}
	if !exists {
		previous, err := i.queryScheme(ctx, "pixiv")
		if err != nil {
			return err
		}
		manifest = handlerManifest{Version: 1, ExecutablePath: executablePath, PreviousHandler: previous}
	} else {
		// upgrade 或二进制路径变化时只刷新启动目标，保留初次接管时的旧 handler。
		manifest.ExecutablePath = executablePath
	}
	manifest.HomeDirectory = homeDirectory
	// LaunchServices 启动的 app 不会继承调用 CLI 的 HOME。将非敏感启动
	// manifest 写入已注册的 bundle，确保隔离 HOME 仍可把 callback 交给对应
	// 的 CLI。
	if err := saveHandlerBundleManifest(appPath, manifest); err != nil {
		return err
	}
	if err := i.registerApp(ctx, appPath); err != nil {
		return err
	}
	if err := i.setScheme(ctx, "pixiv", pixivURLHandlerBundleID); err != nil {
		return err
	}
	if err := saveHandlerManifest(manifest); err != nil {
		// manifest 写入失败时尽力恢复，避免留下无法由 pixiv-cli 清理的默认关联。
		if manifest.PreviousHandler != "" && manifest.PreviousHandler != pixivURLHandlerBundleID {
			_ = i.setScheme(context.Background(), "pixiv", manifest.PreviousHandler)
		}
		return err
	}
	return nil
}

// delegateToPrevious 精确定向启动此前记录的 macOS bundle。它不通过 shell
// 拼接 URL；无旧 handler 时向系统调用方返回真实错误。
func (i urlHandlerInstaller) delegateToPrevious(ctx context.Context, rawURL string) error {
	manifest, exists, err := loadHandlerManifest()
	if err != nil {
		return err
	}
	if !exists || manifest.PreviousHandler == "" || manifest.PreviousHandler == pixivURLHandlerBundleID {
		return errors.New("no previous Pixiv URL handler is available")
	}
	if err := i.openPrevious(ctx, manifest.PreviousHandler, rawURL); err != nil {
		return errors.New("could not open previous Pixiv URL handler")
	}
	return nil
}

// install 为本次登录安装 pixiv:// 回调 helper，并返回清理函数。
func (i urlHandlerInstaller) install(ctx context.Context, callbackRelayURL string) (func(), error) {
	if _, err := i.lookPath("swiftc"); err != nil {
		return nil, err
	}
	if _, err := i.lookPath("swift"); err != nil {
		return nil, err
	}
	appPath, err := pixivURLHandlerAppPath()
	if err != nil {
		return nil, err
	}
	if err := i.ensureApp(ctx, appPath); err != nil {
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
	previous, _ := i.queryScheme(ctx, "pixiv")
	if err := i.registerApp(ctx, appPath); err != nil {
		return nil, err
	}
	if err := i.setScheme(ctx, "pixiv", pixivURLHandlerBundleID); err != nil {
		if previous != "" && previous != pixivURLHandlerBundleID {
			_ = i.setScheme(context.Background(), "pixiv", previous)
		}
		return nil, err
	}
	installed = true
	cleanup := func() {
		_ = os.Remove(endpointPath)
		if previous != "" && previous != pixivURLHandlerBundleID {
			_ = i.setScheme(context.Background(), "pixiv", previous)
		}
	}
	return cleanup, nil
}

// pixivURLHandlerAppPath 返回当前用户私有 app 数据目录下 helper app 的位置。
func pixivURLHandlerAppPath() (string, error) {
	appDataDir, err := paths.UserDataSubdir(paths.AppDataDirName)
	if err != nil {
		return "", err
	}
	return filepath.Join(appDataDir, "url-handler", "PixivCLIURLHandler.app"), nil
}

func handlerBundleManifestPath(appPath string) string {
	return filepath.Join(appPath, "Contents", "Resources", handlerManifestFilename)
}

// saveHandlerBundleManifest 保存 helper 唯一需要的启动目标。manifest 不含
// callback、token 或配对私钥，且 bundle 位于当前用户的私有应用数据目录。
func saveHandlerBundleManifest(appPath string, manifest handlerManifest) error {
	body, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return filesecret.WritePrivateFile(handlerBundleManifestPath(appPath), body, paths.PrivateFileMode)
}

// ensureApp 编译并安装当前用户的 macOS pixiv:// helper app。
func (i urlHandlerInstaller) ensureApp(ctx context.Context, appPath string) error {
	executablePath := filepath.Join(appPath, "Contents", "MacOS", "PixivCLIURLHandler")
	infoPath := filepath.Join(appPath, "Contents", "Info.plist")
	versionPath := filepath.Join(appPath, "Contents", "Resources", "source-version")
	if fileExists(executablePath) && fileExists(infoPath) && handlerSourceVersionMatches(versionPath) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(executablePath), paths.PrivateDirMode); err != nil {
		return err
	}
	// Swift 源码可能包含后续敏感回调逻辑；私有随机目录和独占创建共同避免固定路径的 symlink 覆盖及并发竞态。
	sourceDir, err := os.MkdirTemp("", "pixiv-cli-url-handler-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(sourceDir)
	if err := os.Chmod(sourceDir, paths.PrivateDirMode); err != nil {
		return err
	}
	source, err := os.CreateTemp(sourceDir, "url-handler-*.swift")
	if err != nil {
		return err
	}
	sourcePath := source.Name()
	if err := source.Chmod(paths.PrivateFileMode); err != nil {
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
	if out, err := i.compile(ctx, sourcePath, executablePath); err != nil {
		return fmt.Errorf("compile pixiv:// callback helper: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if err := os.WriteFile(infoPath, []byte(pixivURLHandlerInfoPlist), paths.PrivateFileMode); err != nil {
		return err
	}
	if err := filesecret.WritePrivateFile(versionPath, []byte(pixivURLHandlerSourceVersion+"\n"), paths.PrivateFileMode); err != nil {
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

// pixivURLHandlerSwiftSource 是随 helper app 编译的 Swift 源码。它只读取
// manifest 记录的 binary 路径并回调 CLI，不接触网络或任意本地文件。
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
		if let homeDirectory = object["home_directory"] as? String, !homeDirectory.isEmpty {
			var environment = ProcessInfo.processInfo.environment
			environment["HOME"] = homeDirectory
			environment["USERPROFILE"] = homeDirectory
			process.environment = environment
		}
		try? process.run()
	}

	private func manifestURL() -> URL? {
		// LaunchServices 不会继承 CLI 进程的 HOME，因此只读取随已注册 app
		// 一同写入的私有 manifest；它不含 bearer secret。
		guard let resourceURL = Bundle.main.resourceURL else {
			return nil
		}
		return resourceURL.appendingPathComponent("handler-manifest.json")
	}
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.setActivationPolicy(.accessory)
app.run()
`

// Install 为本次登录安装 pixiv:// 回调 helper，并返回清理函数。
func Install(ctx context.Context, callbackRelayURL string) (func(), error) {
	return newURLHandlerInstaller().install(ctx, callbackRelayURL)
}
