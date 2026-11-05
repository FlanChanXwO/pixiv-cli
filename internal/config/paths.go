package config

import (
	"os"
	"path/filepath"
)

const (
	// AppDataDirName 指定本项目所有本机持久状态的唯一根目录名。
	AppDataDirName = ".pixiv-cli"
	// PrivateDirMode 指定私有目录的权限位。
	PrivateDirMode = 0o700
	// PrivateFileMode 指定私有文件的权限位。
	PrivateFileMode = 0o600
)

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

// ConfigFilePath 返回 config.toml 的应用管理路径。
func ConfigFilePath() (string, error) {
	directory, err := AppDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "config.toml"), nil
}
