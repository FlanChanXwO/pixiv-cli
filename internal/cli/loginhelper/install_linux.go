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

	"github.com/FlanChanXwO/pixiv-cli/internal/common/constants"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
)

const linuxURLHandlerDesktopFile = "pixiv-cli-url-handler.desktop"
const linuxPixivURLScheme = "x-scheme-handler/pixiv"

var (
	linuxExecutablePath = os.Executable
	runLinuxXDGMIme     = defaultRunLinuxXDGMIme
)

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

type linuxFileSnapshot struct {
	path    string
	exists  bool
	mode    os.FileMode
	content []byte
}

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
		return linuxFileSnapshot{path: path}, nil
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
	return linuxFileSnapshot{path: path, exists: true, mode: info.Mode().Perm(), content: content}, nil
}

func restoreLinuxMimeState(snapshots []linuxFileSnapshot) error {
	var errs []error
	for _, snapshot := range snapshots {
		if !snapshot.exists {
			if err := os.Remove(snapshot.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(snapshot.path), constants.PrivateDirMode); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := os.WriteFile(snapshot.path, snapshot.content, snapshot.mode); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := os.Chmod(snapshot.path, snapshot.mode); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
