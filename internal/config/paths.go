package config

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/common/constants"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
)

const DefaultConfigFileMode = constants.PrivateFileMode

var configFilePath = defaultConfigFilePath

func defaultAppConfigDir() (string, error) {
	return files.UserDataSubdir(constants.AppDataDirName)
}

func defaultConfigFilePath() (string, error) {
	return files.UserDataFile(constants.AppDataDirName, "config.toml")
}

func ConfigFilePath() (string, error) {
	return configFilePath()
}

func SetFilePathForTest(configPath string) func() {
	oldConfig := configFilePath
	configFilePath = func() (string, error) { return configPath, nil }
	return func() {
		configFilePath = oldConfig
	}
}

func WritePrivateFile(path string, body []byte) error {
	return files.WritePrivateFile(path, body, DefaultConfigFileMode)
}
