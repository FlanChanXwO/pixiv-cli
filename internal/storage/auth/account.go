package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type AuthStore struct {
	DefaultAccount string    `json:"default_account,omitempty"`
	Accounts       []Account `json:"accounts,omitempty"`
}

type Account struct {
	Name         string `json:"name"`
	RefreshToken string `json:"refresh_token"`
	UserID       int64  `json:"user_id,omitempty"`
}

func LoadAuthStore(path string) (AuthStore, error) {
	store := AuthStore{Accounts: []Account{}}
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
		store.Accounts = []Account{}
	}
	if store.DefaultAccount != "" && !store.Has(store.DefaultAccount) {
		store.DefaultAccount = ""
	}
	return store, nil
}

func SaveAuthStore(path string, store AuthStore) error {
	if store.Accounts == nil {
		store.Accounts = []Account{}
	}
	body, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return WritePrivateFile(path, body)
}

func (s AuthStore) Names() []string {
	names := make([]string, 0, len(s.Accounts))
	for _, acct := range s.Accounts {
		names = append(names, acct.Name)
	}
	return names
}

func ValidateAccountName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("Account name cannot be empty")
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("Account name %q cannot contain path separators", name)
	}
	return nil
}

func (s AuthStore) Has(name string) bool {
	_, _, ok := s.Get(name)
	return ok
}

func (s AuthStore) Get(name string) (int, Account, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return -1, Account{}, false
	}
	for idx, acct := range s.Accounts {
		if acct.Name == name {
			return idx, acct, true
		}
	}
	return -1, Account{}, false
}

func (s *AuthStore) Upsert(acct Account) {
	if idx, _, ok := s.Get(acct.Name); ok {
		s.Accounts[idx] = acct
		return
	}
	s.Accounts = append(s.Accounts, acct)
}

func (s *AuthStore) Remove(name string) bool {
	idx, _, ok := s.Get(name)
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

func SelectAuthAccount(store AuthStore, requestedName string) (string, Account, bool) {
	name := strings.TrimSpace(requestedName)
	if name == "" {
		name = store.DefaultAccount
	}
	idx, acct, ok := store.Get(name)
	if !ok {
		return "", Account{}, false
	}
	return store.Accounts[idx].Name, acct, true
}
