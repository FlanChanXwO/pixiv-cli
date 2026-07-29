//go:build linux

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

const linuxURLHandlerDesktopFile = "pixiv-cli-url-handler.desktop"
const linuxPixivURLScheme = "x-scheme-handler/pixiv"

var (
	linuxExecutablePath      = os.Executable
	runLinuxXDGMIme          = defaultRunLinuxXDGMIme
	runLinuxGioLaunch        = defaultRunLinuxGioLaunch
	findLinuxCommand         = exec.LookPath
	queryLinuxDefaultHandler = linuxDefaultURLHandler
)

// EnsurePersistent 安装桌面 Linux 的按需 callback handler。headless Linux
// 不会调用它；server relay 本身不依赖任何桌面集成。
func EnsurePersistent(ctx context.Context) error {
	if _, err := findLinuxCommand("xdg-mime"); err != nil {
		return errors.New("xdg-mime is required for the desktop Pixiv callback handler")
	}
	if _, err := findLinuxCommand("gio"); err != nil {
		return errors.New("gio is required for the desktop Pixiv callback handler")
	}
	executablePath, err := linuxExecutablePath()
	if err != nil {
		return err
	}
	applicationsDir, err := linuxApplicationsDir()
	if err != nil {
		return err
	}
	desktopPath := filepath.Join(applicationsDir, linuxURLHandlerDesktopFile)
	manifest, exists, err := loadHandlerManifest()
	if err != nil {
		return err
	}
	var snapshots []linuxFileSnapshot
	if !exists {
		previous, err := queryLinuxDefaultHandler(ctx)
		if err != nil {
			return err
		}
		snapshots, err = snapshotLinuxMimeState(applicationsDir, desktopPath)
		if err != nil {
			return err
		}
		manifest = handlerManifest{Version: 1, ExecutablePath: executablePath, PreviousHandler: previous, LinuxMIMESnapshots: snapshots}
	} else {
		manifest.ExecutablePath = executablePath
		snapshots = manifest.LinuxMIMESnapshots
	}
	if err := files.WritePrivateFile(desktopPath, []byte(linuxDesktopEntry(executablePath)), constants.PrivateFileMode); err != nil {
		return err
	}
	if err := runLinuxXDGMIme(ctx, "default", linuxURLHandlerDesktopFile, linuxPixivURLScheme); err != nil {
		if len(snapshots) > 0 {
			_ = restoreLinuxMimeState(snapshots)
		}
		return err
	}
	if err := saveHandlerManifest(manifest); err != nil {
		if len(snapshots) > 0 {
			_ = restoreLinuxMimeState(snapshots)
		}
		return err
	}
	return nil
}

func DisablePersistent(ctx context.Context) error {
	manifest, exists, err := loadHandlerManifest()
	if err != nil || !exists {
		return err
	}
	current, err := queryLinuxDefaultHandler(ctx)
	if err != nil {
		return err
	}
	if current == linuxURLHandlerDesktopFile {
		if len(manifest.LinuxMIMESnapshots) > 0 {
			if err := restoreLinuxMimeState(manifest.LinuxMIMESnapshots); err != nil {
				return err
			}
		} else if manifest.PreviousHandler != "" && manifest.PreviousHandler != linuxURLHandlerDesktopFile {
			// 兼容没有 snapshot 的旧 manifest：至少恢复已记录的 handler。
			if err := runLinuxXDGMIme(ctx, "default", manifest.PreviousHandler, linuxPixivURLScheme); err != nil {
				return err
			}
			applicationsDir, err := linuxApplicationsDir()
			if err != nil {
				return err
			}
			if err := os.Remove(filepath.Join(applicationsDir, linuxURLHandlerDesktopFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		} else if manifest.PreviousHandler == "" {
			return errors.New("cannot safely restore a previous Linux Pixiv URL handler")
		}
	}
	return removeHandlerManifest()
}

func DelegateToPrevious(ctx context.Context, rawURL string) error {
	manifest, exists, err := loadHandlerManifest()
	if err != nil {
		return err
	}
	if !exists || manifest.PreviousHandler == "" || manifest.PreviousHandler == linuxURLHandlerDesktopFile {
		return errors.New("no previous Pixiv URL handler is available")
	}
	if _, err := findLinuxCommand("gio"); err != nil {
		return errors.New("gio is required to open the previous Pixiv URL handler")
	}
	desktopPath, err := linuxDesktopFilePath(manifest.PreviousHandler)
	if err != nil {
		return errors.New("could not locate previous Pixiv URL handler")
	}
	// gio launch 要求实际 desktop-file 路径而不是 XDG desktop ID；仍以参数
	// 数组启动，callback URL 不会经过 shell 解释。
	if err := runLinuxGioLaunch(ctx, desktopPath, rawURL); err != nil {
		return errors.New("could not open previous Pixiv URL handler")
	}
	return nil
}

func defaultRunLinuxGioLaunch(ctx context.Context, desktopPath, rawURL string) error {
	return exec.CommandContext(ctx, "gio", "launch", desktopPath, rawURL).Run()
}

func linuxDefaultURLHandler(ctx context.Context) (string, error) {
	command := exec.CommandContext(ctx, "xdg-mime", "query", "default", linuxPixivURLScheme)
	out, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("query xdg-mime: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Install 在桌面 Linux 上只为当前 auth login 尝试注册 pixiv:// handler。
// xdg-mime 会写入用户 mimeapps 文件，因此在修改前保留所有可能的用户级位置并在
// cleanup 时逐字节恢复，避免没有原默认 handler 的用户留下悬挂关联。
func Install(ctx context.Context, callbackRelayURL string) (cleanup func(), err error) {
	endpointPath, err := writeCallbackEndpoint(callbackRelayURL)
	if err != nil {
		return nil, err
	}
	cleanupEndpoint := func() { _ = os.Remove(endpointPath) }
	installed := false
	defer func() {
		if !installed {
			cleanupEndpoint()
		}
	}()

	executablePath, err := linuxExecutablePath()
	if err != nil {
		return nil, err
	}
	applicationsDir, err := linuxApplicationsDir()
	if err != nil {
		return nil, err
	}
	desktopPath := filepath.Join(applicationsDir, linuxURLHandlerDesktopFile)
	snapshots, err := snapshotLinuxMimeState(applicationsDir, desktopPath)
	if err != nil {
		return nil, err
	}
	if err := files.WritePrivateFile(desktopPath, []byte(linuxDesktopEntry(executablePath)), constants.PrivateFileMode); err != nil {
		return nil, err
	}
	if err := runLinuxXDGMIme(ctx, "default", linuxURLHandlerDesktopFile, linuxPixivURLScheme); err != nil {
		_ = restoreLinuxMimeState(snapshots)
		return nil, err
	}

	installed = true
	return func() {
		cleanupEndpoint()
		_ = restoreLinuxMimeState(snapshots)
	}, nil
}

func defaultRunLinuxXDGMIme(ctx context.Context, args ...string) error {
	command := exec.CommandContext(ctx, "xdg-mime", args...)
	if err := command.Run(); err != nil {
		// 不拼接 command output；desktop 工具的诊断可能包含本机路径或环境。
		return fmt.Errorf("run xdg-mime: %w", err)
	}
	return nil
}

func linuxApplicationsDir() (string, error) {
	dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "applications"), nil
}

// linuxDesktopFilePath 从 xdg-mime 返回的 desktop ID 解析实际文件。gio launch
// 不能接受 ID；同时拒绝路径成分，避免损坏的 mimeapps 状态被当作任意本地文件启动。
func linuxDesktopFilePath(desktopID string) (string, error) {
	desktopID = strings.TrimSpace(desktopID)
	if desktopID == "" || filepath.Base(desktopID) != desktopID || !strings.HasSuffix(strings.ToLower(desktopID), ".desktop") {
		return "", errors.New("invalid desktop handler ID")
	}
	applicationsDir, err := linuxApplicationsDir()
	if err != nil {
		return "", err
	}
	directories := []string{applicationsDir}
	dataDirs := strings.TrimSpace(os.Getenv("XDG_DATA_DIRS"))
	if dataDirs == "" {
		dataDirs = "/usr/local/share:/usr/share"
	}
	for _, dataDir := range strings.Split(dataDirs, ":") {
		dataDir = strings.TrimSpace(dataDir)
		if dataDir != "" {
			directories = append(directories, filepath.Join(dataDir, "applications"))
		}
	}
	seen := make(map[string]struct{}, len(directories))
	for _, directory := range directories {
		directory = filepath.Clean(directory)
		if _, exists := seen[directory]; exists {
			continue
		}
		seen[directory] = struct{}{}
		candidate := filepath.Join(directory, desktopID)
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return "", os.ErrNotExist
}

func linuxDesktopEntry(executablePath string) string {
	return "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=Pixiv CLI OAuth Callback\n" +
		"NoDisplay=true\n" +
		"Exec=" + linuxDesktopExecArgument(executablePath) + " auth " + CallbackCommand + " %u\n" +
		"MimeType=" + linuxPixivURLScheme + ";\n"
}

// linuxDesktopExecArgument 实现 Desktop Entry 的双引号转义子集。当前二进制路径
// 可能带空格；绝不能让路径内容变成 shell 语法或额外参数。
func linuxDesktopExecArgument(argument string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", `\$`, "`", "\\`")
	return `"` + replacer.Replace(argument) + `"`
}

type linuxFileSnapshot = handlerFileSnapshot

func snapshotLinuxMimeState(applicationsDir, desktopPath string) ([]linuxFileSnapshot, error) {
	paths := append(linuxMimeAppsPaths(), desktopPath)
	seen := make(map[string]struct{}, len(paths))
	snapshots := make([]linuxFileSnapshot, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		snapshot, err := snapshotLinuxFile(path)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

func linuxMimeAppsPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	return []string{
		filepath.Join(configHome, "mimeapps.list"),
		filepath.Join(dataHome, "applications", "mimeapps.list"),
	}
}

func snapshotLinuxFile(path string) (linuxFileSnapshot, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return linuxFileSnapshot{Path: path}, nil
	}
	if err != nil {
		return linuxFileSnapshot{}, err
	}
	if !info.Mode().IsRegular() {
		return linuxFileSnapshot{}, errors.New("desktop integration state is not a regular file")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return linuxFileSnapshot{}, err
	}
	return linuxFileSnapshot{Path: path, Exists: true, Mode: info.Mode().Perm(), Content: content}, nil
}

func restoreLinuxMimeState(snapshots []linuxFileSnapshot) error {
	var errs []error
	for _, snapshot := range snapshots {
		if !snapshot.Exists {
			if err := os.Remove(snapshot.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(snapshot.Path), constants.PrivateDirMode); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := os.WriteFile(snapshot.Path, snapshot.Content, snapshot.Mode); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := os.Chmod(snapshot.Path, snapshot.Mode); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
