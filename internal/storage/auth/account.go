package auth

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

type AuthStore struct {
	DefaultUserID int64     `json:"default_user_id,omitempty"`
	Accounts      []Account `json:"accounts,omitempty"`
}

type Account struct {
	RefreshToken string `json:"refresh_token"`
	UserID       int64  `json:"user_id,omitempty"`
	Username     string `json:"username,omitempty"`
}

type authStoreReadHook struct {
	path string
	read func(string) ([]byte, error)
}

var (
	authStoreReadHookState atomic.Pointer[authStoreReadHook]
	authStoreReadHookMu    sync.Mutex
)

// SetReadAuthStoreFileForTest 仅为跨平台本地状态错误测试拦截指定 auth path。
// 其他路径始终使用 os.ReadFile。hook 生命周期由互斥锁串行化，同一路径测试不得
// 并行；调用方必须用 t.Cleanup 执行返回的恢复函数。
func SetReadAuthStoreFileForTest(path string, read func(string) ([]byte, error)) func() {
	if strings.TrimSpace(path) == "" || read == nil {
		panic("auth store read test hook requires a path and reader")
	}
	authStoreReadHookMu.Lock()
	authStoreReadHookState.Store(&authStoreReadHook{path: path, read: read})
	var once sync.Once
	return func() {
		once.Do(func() {
			authStoreReadHookState.Store(nil)
			authStoreReadHookMu.Unlock()
		})
	}
}

func readAuthStore(path string) ([]byte, error) {
	if hook := authStoreReadHookState.Load(); hook != nil && path == hook.path {
		return hook.read(path)
	}
	return os.ReadFile(path)
}

type LegacySchemaError struct {
	Field string
}

type missingRefreshTokenError struct{}

func (missingRefreshTokenError) Error() string { return "auth account refresh_token is required" }

// IsMissingRefreshTokenError 只暴露安全分类，不携带 token 或文件内容。
func IsMissingRefreshTokenError(err error) bool {
	var target missingRefreshTokenError
	return errors.As(err, &target)
}

func (e LegacySchemaError) Error() string {
	return "legacy auth schema field " + e.Field + " is not supported; recreate auth.json with pixiv auth add/login"
}

func IsLegacySchemaError(err error) bool {
	var legacy LegacySchemaError
	return errors.As(err, &legacy)
}

func LoadAuthStore(path string) (AuthStore, error) {
	store := AuthStore{Accounts: []Account{}}
	body, err := readAuthStore(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return store, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return store, nil
	}
	if err := rejectLegacyAuthSchema(body); err != nil {
		return store, err
	}
	if err := json.Unmarshal(body, &store); err != nil {
		return store, err
	}
	if store.Accounts == nil {
		store.Accounts = []Account{}
	}
	if store.DefaultUserID != 0 && !store.Has(store.DefaultUserID) {
		store.DefaultUserID = 0
	}
	if err := validateAuthStore(store, false); err != nil {
		return store, err
	}
	return store, nil
}

func rejectLegacyAuthSchema(body []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	if _, ok := raw["default_account"]; ok {
		return LegacySchemaError{Field: "default_account"}
	}
	accountsBody, ok := raw["accounts"]
	if !ok {
		return nil
	}
	var accounts []map[string]json.RawMessage
	if err := json.Unmarshal(accountsBody, &accounts); err != nil {
		return err
	}
	for _, acct := range accounts {
		if _, ok := acct["name"]; ok {
			return LegacySchemaError{Field: "accounts[].name"}
		}
	}
	return nil
}

func SaveAuthStore(path string, store AuthStore) error {
	if store.Accounts == nil {
		store.Accounts = []Account{}
	}
	if err := validateAuthStore(store, true); err != nil {
		return err
	}
	body, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return WritePrivateFile(path, body)
}

func validateAuthStore(store AuthStore, requireDefault bool) error {
	seen := map[int64]struct{}{}
	for _, acct := range store.Accounts {
		if acct.UserID <= 0 {
			return errors.New("auth account user_id is required; recreate auth.json with pixiv auth add/login")
		}
		if _, ok := seen[acct.UserID]; ok {
			return errors.New("auth account user_id values must be unique")
		}
		seen[acct.UserID] = struct{}{}
		if strings.TrimSpace(acct.RefreshToken) == "" {
			return missingRefreshTokenError{}
		}
	}
	if len(store.Accounts) > 0 && requireDefault && store.DefaultUserID == 0 {
		return errors.New("auth default_user_id is required when accounts are present")
	}
	if store.DefaultUserID != 0 {
		if _, ok := seen[store.DefaultUserID]; !ok {
			return errors.New("auth default_user_id must reference an existing account")
		}
	}
	return nil
}

func (s AuthStore) UserIDs() []int64 {
	userIDs := make([]int64, 0, len(s.Accounts))
	for _, acct := range s.Accounts {
		userIDs = append(userIDs, acct.UserID)
	}
	return userIDs
}

func (s AuthStore) Has(userID int64) bool {
	_, _, ok := s.Get(userID)
	return ok
}

func (s AuthStore) Get(userID int64) (int, Account, bool) {
	if userID == 0 {
		return -1, Account{}, false
	}
	for idx, acct := range s.Accounts {
		if acct.UserID == userID {
			return idx, acct, true
		}
	}
	return -1, Account{}, false
}

func (s *AuthStore) Upsert(acct Account) {
	if idx, _, ok := s.Get(acct.UserID); ok {
		s.Accounts[idx] = acct
		return
	}
	s.Accounts = append(s.Accounts, acct)
}

func (s *AuthStore) Remove(userID int64) bool {
	idx, _, ok := s.Get(userID)
	if !ok {
		return false
	}
	s.Accounts = append(s.Accounts[:idx], s.Accounts[idx+1:]...)
	if s.DefaultUserID == userID {
		s.DefaultUserID = 0
		if len(s.Accounts) > 0 {
			s.DefaultUserID = s.Accounts[0].UserID
		}
	}
	return true
}

func SelectAuthAccount(store AuthStore, requestedUserID int64) (int64, Account, bool) {
	userID := requestedUserID
	if userID == 0 {
		userID = store.DefaultUserID
	}
	idx, acct, ok := store.Get(userID)
	if !ok {
		return 0, Account{}, false
	}
	return store.Accounts[idx].UserID, acct, true
}
