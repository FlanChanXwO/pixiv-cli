package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type authStore struct {
	DefaultAccount string    `json:"default_account,omitempty"`
	Accounts       []account `json:"accounts,omitempty"`
}

type account struct {
	Name         string `json:"name"`
	RefreshToken string `json:"refresh_token"`
	UserID       int64  `json:"user_id,omitempty"`
}

func loadAuthStore(path string) (authStore, error) {
	store := authStore{Accounts: []account{}}
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
		store.Accounts = []account{}
	}
	if store.DefaultAccount != "" && !store.has(store.DefaultAccount) {
		store.DefaultAccount = ""
	}
	return store, nil
}

func saveAuthStore(path string, store authStore) error {
	if store.Accounts == nil {
		store.Accounts = []account{}
	}
	body, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return writePrivateFile(path, body)
}

func (s authStore) names() []string {
	names := make([]string, 0, len(s.Accounts))
	for _, acct := range s.Accounts {
		names = append(names, acct.Name)
	}
	return names
}

func validateAccountName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("account name cannot be empty")
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("account name %q cannot contain path separators", name)
	}
	return nil
}

func (s authStore) has(name string) bool {
	_, _, ok := s.get(name)
	return ok
}

func (s authStore) get(name string) (int, account, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return -1, account{}, false
	}
	for idx, acct := range s.Accounts {
		if acct.Name == name {
			return idx, acct, true
		}
	}
	return -1, account{}, false
}

func (s *authStore) upsert(acct account) {
	if idx, _, ok := s.get(acct.Name); ok {
		s.Accounts[idx] = acct
		return
	}
	s.Accounts = append(s.Accounts, acct)
}

func (s *authStore) remove(name string) bool {
	idx, _, ok := s.get(name)
	if !ok {
		return false
	}
	s.Accounts = append(s.Accounts[:idx], s.Accounts[idx+1:]...)
	if s.DefaultAccount == name {
		s.DefaultAccount = ""
		if len(s.Accounts) > 0 {
			s.DefaultAccount = s.Accounts[0].Name
		}
	}
	return true
}

func selectAuthAccount(store authStore, requestedName string) (string, account, bool) {
	name := strings.TrimSpace(requestedName)
	if name == "" {
		name = store.DefaultAccount
	}
	idx, acct, ok := store.get(name)
	if !ok {
		return "", account{}, false
	}
	return store.Accounts[idx].Name, acct, true
}
