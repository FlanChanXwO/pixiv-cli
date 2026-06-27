package cmd

import (
	"os"
	"path/filepath"
)

const defaultConfigFileMode = 0o600

var (
	authFilePath   = defaultAuthFilePath
	configFilePath = defaultConfigFilePath
)

func defaultAppConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pixiv"), nil
}

func defaultAuthFilePath() (string, error) {
	dir, err := defaultAppConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "auth.json"), nil
}

func defaultConfigFilePath() (string, error) {
	dir, err := defaultAppConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

func writePrivateFile(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, body, defaultConfigFileMode); err != nil {
		return err
	}
	return os.Chmod(path, defaultConfigFileMode)
}
