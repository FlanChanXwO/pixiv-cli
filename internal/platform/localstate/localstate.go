// Package localstate 定义本机私有状态的路径命名空间与最小权限要求。
// 它不包含 Pixiv 协议、MCP 传输或业务配置解析。
package localstate

import (
	"os"
	"path/filepath"
)

const (
	// AppDataDirName 是本项目所有本机持久状态的唯一根目录名。
	AppDataDirName  = ".pixiv-cli"
	PrivateDirMode  = 0o700
	PrivateFileMode = 0o600
)

var configFilePathOverride func() (string, error)

// UserDataSubdir 返回当前用户 home 下的应用数据目录。所有平台均使用相同的
// `~/APP_NAME` 语义；Windows 上的 home 由 os.UserHomeDir 解析为用户 profile。
func UserDataSubdir(appName string) (string, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appName), nil
}

// AppDataDir 返回本项目当前用户的私有应用数据根目录。
func AppDataDir() (string, error) {
	return UserDataSubdir(AppDataDirName)
}

// UserDataFile 返回应用数据目录内的指定文件路径。
func UserDataFile(appName, filename string) (string, error) {
	dir, err := UserDataSubdir(appName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filename), nil
}

// ConfigFilePath 返回 config.toml 的 app-managed 路径。配置 schema 由
// storage/config 拥有；本机路径与权限只由 localstate 解析。
func ConfigFilePath() (string, error) {
	if configFilePathOverride != nil {
		return configFilePathOverride()
	}
	directory, err := AppDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "config.toml"), nil
}

// SetConfigFilePathForTest 为依赖默认 app path 的进程测试提供可恢复的路径
// 注入。生产代码不调用它；恢复函数必须在测试清理阶段执行。
func SetConfigFilePathForTest(path string) func() {
	previous := configFilePathOverride
	configFilePathOverride = func() (string, error) { return path, nil }
	return func() { configFilePathOverride = previous }
}
