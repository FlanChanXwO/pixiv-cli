package state

import (
	"os"
	"path/filepath"
)

const DefaultAuthFileMode = 0o600

var (
	authFilePath = defaultAuthFilePath
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
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, body, DefaultAuthFileMode); err != nil {
		return err
	}
	return os.Chmod(path, DefaultAuthFileMode)
}
