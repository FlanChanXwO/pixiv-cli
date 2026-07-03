package files

import (
	"os"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-mcp-server/internal/common/constants"
)

func UserConfigSubdir(appName string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appName), nil
}

func UserConfigFile(appName, filename string) (string, error) {
	dir, err := UserConfigSubdir(appName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filename), nil
}

func WritePrivateFile(path string, body []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), constants.PrivateDirMode); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Dir(path), constants.PrivateDirMode); err != nil {
		return err
	}
	if err := os.WriteFile(path, body, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}
