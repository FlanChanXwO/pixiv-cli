package config

import (
	"os"
	"path/filepath"
)

const DefaultConfigFileMode = 0o600

var configFilePath = defaultConfigFilePath

func defaultAppConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pixiv"), nil
}

func defaultConfigFilePath() (string, error) {
	dir, err := defaultAppConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
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
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, body, DefaultConfigFileMode); err != nil {
		return err
	}
	return os.Chmod(path, DefaultConfigFileMode)
}
