package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const defaultConfigFileMode = 0o600

var configPath = defaultConfigPath

type accountStore struct {
	DefaultProfile string             `json:"default_profile,omitempty"`
	Accounts       map[string]account `json:"accounts"`
}

type account struct {
	RefreshToken string `json:"refresh_token"`
	UserID       int64  `json:"user_id,omitempty"`
}

func defaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pixiv", "config.json"), nil
}

func loadAccountStore(path string) (accountStore, error) {
	store := accountStore{Accounts: map[string]account{}}
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return store, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return store, nil
	}
	if err := json.Unmarshal(body, &store); err != nil {
		return store, err
	}
	if store.Accounts == nil {
		store.Accounts = map[string]account{}
	}
	return store, nil
}

func saveAccountStore(path string, store accountStore) error {
	if store.Accounts == nil {
		store.Accounts = map[string]account{}
	}
	body, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, body, defaultConfigFileMode); err != nil {
		return err
	}
	return os.Chmod(path, defaultConfigFileMode)
}

func profileNames(store accountStore) []string {
	names := make([]string, 0, len(store.Accounts))
	for name := range store.Accounts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func validateProfileName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("profile name cannot be empty")
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("profile name %q cannot contain path separators", name)
	}
	return nil
}

func selectAccount(store accountStore, profile string) (string, account, bool) {
	name := strings.TrimSpace(profile)
	if name == "" {
		name = store.DefaultProfile
	}
	if name == "" {
		return "", account{}, false
	}
	acct, ok := store.Accounts[name]
	return name, acct, ok
}
