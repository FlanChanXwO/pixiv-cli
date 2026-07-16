//go:build darwin

package loginhelper

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/common/constants"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
)

const pixivURLHandlerBundleID = "com.flanchan.pixiv-cli.url-handler"

// Install 为本次登录安装 pixiv:// 回调 helper，并返回清理函数。
func Install(ctx context.Context, manualURL string) (func(), error) {
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
	endpointPath, err := pixivURLHandlerEndpointPath()
	if err != nil {
		return nil, err
	}
	if err := files.WritePrivateFile(endpointPath, []byte(strings.TrimSpace(manualURL)+"\n"), constants.PrivateFileMode); err != nil {
		return nil, err
	}
	previous, _ := defaultURLSchemeHandler(ctx, "pixiv")
	if err := registerURLHandlerApp(ctx, appPath); err != nil {
		return nil, err
	}
	if err := setDefaultURLSchemeHandler(ctx, "pixiv", pixivURLHandlerBundleID); err != nil {
		return nil, err
	}
	cleanup := func() {
		_ = os.Remove(endpointPath)
		if previous != "" && previous != pixivURLHandlerBundleID {
			_ = setDefaultURLSchemeHandler(context.Background(), "pixiv", previous)
		}
	}
	return cleanup, nil
}

func pixivURLHandlerAppPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Applications", "PixivCLIURLHandler.app"), nil
}

func pixivURLHandlerEndpointPath() (string, error) {
	dir, err := files.UserConfigSubdir(constants.AppConfigDirName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "url-handler-endpoint"), nil
}

func ensurePixivURLHandlerApp(ctx context.Context, appPath string) error {
	executablePath := filepath.Join(appPath, "Contents", "MacOS", "PixivCLIURLHandler")
	infoPath := filepath.Join(appPath, "Contents", "Info.plist")
	if fileExists(executablePath) && fileExists(infoPath) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(executablePath), constants.PrivateDirMode); err != nil {
		return err
	}
	sourcePath := filepath.Join(os.TempDir(), "pixiv-cli-url-handler.swift")
	if err := os.WriteFile(sourcePath, []byte(pixivURLHandlerSwiftSource), constants.PrivateFileMode); err != nil {
		return err
	}
	defer os.Remove(sourcePath)
	cmd := exec.CommandContext(ctx, "swiftc", sourcePath, "-o", executablePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("compile pixiv:// callback helper: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if err := os.WriteFile(infoPath, []byte(pixivURLHandlerInfoPlist), constants.PrivateFileMode); err != nil {
		return err
	}
	return nil
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
        post(callbackURL)
        NSApp.terminate(nil)
    }

    private func post(_ callbackURL: String) {
        guard let endpointPath = endpointPath(),
              let endpoint = try? String(contentsOfFile: endpointPath, encoding: .utf8).trimmingCharacters(in: .whitespacesAndNewlines),
              let url = URL(string: endpoint),
              !endpoint.isEmpty else {
            return
        }
        var components = URLComponents()
        components.queryItems = [URLQueryItem(name: "code", value: callbackURL)]
        let body = components.percentEncodedQuery ?? ""
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/x-www-form-urlencoded", forHTTPHeaderField: "Content-Type")
        request.httpBody = body.data(using: .utf8)
        let semaphore = DispatchSemaphore(value: 0)
        URLSession.shared.dataTask(with: request) { _, _, _ in
            semaphore.signal()
        }.resume()
        _ = semaphore.wait(timeout: .now() + 10)
    }

    private func endpointPath() -> String? {
        guard let supportDir = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first else {
            return nil
        }
        return supportDir.appendingPathComponent("pixiv/url-handler-endpoint").path
    }
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.setActivationPolicy(.accessory)
app.run()
`
