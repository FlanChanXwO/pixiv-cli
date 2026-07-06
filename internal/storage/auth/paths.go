package auth

import (
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
	return files.WritePrivateFile(path, body, DefaultAuthFileMode)
}
