package auth

import (
	"os"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/internal/common/constants"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
)

const DefaultAuthFileMode = constants.PrivateFileMode

var (
	authFilePath = defaultAuthFilePath
)

func defaultAppConfigDir() (string, error) {
	return files.UserConfigSubdir(constants.AppConfigDirName)
}

func defaultAuthFilePath() (string, error) {
	return files.UserConfigFile(constants.AppConfigDirName, "auth.json")
}

func AuthFilePath() (string, error) {
	return authFilePath()
}

func SetAuthFilePathForTest(authPath string) func() {
	oldAuth := authFilePath
	authFilePath = func() (string, error) { return authPath, nil }
	return func() {
		authFilePath = oldAuth
	}
}

func WritePrivateFile(path string, body []byte) error {
	return writePrivateFile(path, body, files.ReplaceFile)
}

// writePrivateFile keeps the replacement primitive injectable for filesystem
// failure tests while every production caller uses the atomic ReplaceFile path.
func writePrivateFile(path string, body []byte, replaceFile func(string, string) error) error {
	if err := os.MkdirAll(filepath.Dir(path), constants.PrivateDirMode); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Dir(path), constants.PrivateDirMode); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".auth-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	keepTemporary := true
	defer func() {
		if keepTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(DefaultAuthFileMode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replaceFile(temporaryPath, path); err != nil {
		return err
	}
	keepTemporary = false
	return os.Chmod(path, DefaultAuthFileMode)
}
