package pixiv

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
)

const (
	// AuthExportBundleSchema 与 AuthExportBundleVersion 共同标识稳定的认证导出格式。
	AuthExportBundleSchema  = "pixiv-cli.auth-export"
	AuthExportBundleVersion = 1
)

// AuthExportSelection 指定认证导出范围。零值导出本地默认账号；UserID 导出
// 指定账号；All 导出全部账号。
type AuthExportSelection struct {
	UserID int64
	All    bool
}

// AuthExportSecretAccount 是认证导出中的账号记录。
//
// RefreshToken 是 opaque secret；调用方不得记录、诊断输出或意外转发此值。
type AuthExportSecretAccount struct {
	UserID       int64  `json:"user_id"`
	Username     string `json:"username"`
	RefreshToken string `json:"refresh_token"`
}

// AuthExportBundle 是含 opaque refresh-token secret 的版本化认证导出。
type AuthExportBundle struct {
	Schema        string                    `json:"schema"`
	Version       int                       `json:"version"`
	DefaultUserID int64                     `json:"default_user_id"`
	Accounts      []AuthExportSecretAccount `json:"accounts"`
}

// AuthRestoreResult 汇总 restore 结果；账号摘要不含任何 secret。
type AuthRestoreResult struct {
	DefaultUserID int64     `json:"default_user_id"`
	Added         []Account `json:"added"`
	Updated       []Account `json:"updated"`
}

// ExportAuthBundle 从配置的本地 auth store 取得一次只读快照。
func (c *Client) ExportAuthBundle(selection AuthExportSelection) (*AuthExportBundle, error) {
	if selection.UserID < 0 {
		return nil, newError(CodeInvalidArgument, OperationExportAuthBundle, "", false, 0, 0, errors.New("user id must be positive"))
	}
	if selection.All && selection.UserID != 0 {
		return nil, newError(CodeInvalidArgument, OperationExportAuthBundle, "", false, 0, 0, errors.New("all and user id cannot be combined"))
	}
	c.authState.mu.Lock()
	defer c.authState.mu.Unlock()

	authPath := c.authFilePath
	if c.defaults != nil {
		authPath = strings.TrimSpace(c.defaults.options.AuthFilePath)
		if authPath == "" {
			var err error
			authPath, err = auth.AuthFilePath()
			if err != nil {
				return nil, localSnapshotError(OperationExportAuthBundle, markLocalState(localStateStagePath, err))
			}
		}
	}
	if authPath == "" {
		return nil, unsupportedLocalStore(OperationExportAuthBundle, "auth store path was not configured")
	}
	store, err := auth.LoadAuthStore(authPath)
	if err != nil {
		return nil, localSnapshotError(OperationExportAuthBundle, markLocalState(localStateStageAuth, err))
	}
	if selection.All {
		if len(store.Accounts) == 0 || store.DefaultUserID == 0 {
			return nil, newError(CodeUnauthorized, OperationExportAuthBundle, "", false, 0, 0, errors.New("a stored account is required"))
		}
		accounts := make([]AuthExportSecretAccount, len(store.Accounts))
		for i, account := range store.Accounts {
			accounts[i] = AuthExportSecretAccount{
				UserID:       account.UserID,
				Username:     account.Username,
				RefreshToken: account.RefreshToken,
			}
		}
		return &AuthExportBundle{
			Schema:        AuthExportBundleSchema,
			Version:       AuthExportBundleVersion,
			DefaultUserID: store.DefaultUserID,
			Accounts:      accounts,
		}, nil
	}
	selectedUserID := selection.UserID
	if selectedUserID == 0 {
		selectedUserID = store.DefaultUserID
	}
	_, selected, ok := store.Get(selectedUserID)
	if !ok {
		if selection.UserID != 0 {
			return nil, newUserError(CodeInvalidArgument, OperationExportAuthBundle, "", false, 0, selection.UserID, errors.New("account does not exist"))
		}
		return nil, newError(CodeUnauthorized, OperationExportAuthBundle, "", false, 0, 0, errors.New("a stored account is required"))
	}
	return &AuthExportBundle{
		Schema:        AuthExportBundleSchema,
		Version:       AuthExportBundleVersion,
		DefaultUserID: selected.UserID,
		Accounts: []AuthExportSecretAccount{{
			UserID:       selected.UserID,
			Username:     selected.Username,
			RefreshToken: selected.RefreshToken,
		}},
	}, nil
}

// EncodeAuthExportBundle 编码稳定的认证导出 JSON，并保留结尾换行。
func EncodeAuthExportBundle(bundle *AuthExportBundle) ([]byte, error) {
	if err := validateAuthExportBundle(bundle); err != nil {
		return nil, newError(CodeInvalidArgument, OperationEncodeAuthBundle, "", false, 0, 0, err)
	}
	body, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return nil, newError(CodeInvalidArgument, OperationEncodeAuthBundle, "", false, 0, 0, errors.New("auth export bundle cannot be encoded"))
	}
	return append(body, '\n'), nil
}

// DecodeAuthExportBundle 解码认证导出 JSON。
func DecodeAuthExportBundle(body []byte) (*AuthExportBundle, error) {
	var bundle AuthExportBundle
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bundle); err != nil {
		return nil, newError(CodeInvalidArgument, OperationDecodeAuthBundle, "", false, 0, 0, errors.New("auth export bundle is malformed"))
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, newError(CodeInvalidArgument, OperationDecodeAuthBundle, "", false, 0, 0, errors.New("auth export bundle is malformed"))
	}
	if err := validateAuthExportBundle(&bundle); err != nil {
		return nil, newError(CodeInvalidArgument, OperationDecodeAuthBundle, "", false, 0, 0, err)
	}
	return &bundle, nil
}

// RestoreAuthBundle 将已解码的认证导出离线合并到配置的本地 auth store。
func (c *Client) RestoreAuthBundle(bundle *AuthExportBundle) (*AuthRestoreResult, error) {
	if err := validateAuthExportBundle(bundle); err != nil {
		return nil, newError(CodeInvalidArgument, OperationRestoreAuthBundle, "", false, 0, 0, err)
	}
	c.authState.mu.Lock()
	defer c.authState.mu.Unlock()

	authPath := c.authFilePath
	if c.defaults != nil {
		authPath = strings.TrimSpace(c.defaults.options.AuthFilePath)
		if authPath == "" {
			var err error
			authPath, err = auth.AuthFilePath()
			if err != nil {
				return nil, localSnapshotError(OperationRestoreAuthBundle, markLocalState(localStateStagePath, err))
			}
		}
	}
	if authPath == "" {
		return nil, unsupportedLocalStore(OperationRestoreAuthBundle, "auth store path was not configured")
	}
	store, err := auth.LoadAuthStore(authPath)
	if err != nil {
		return nil, localSnapshotError(OperationRestoreAuthBundle, markLocalState(localStateStageAuth, err))
	}
	existingDefault := store.DefaultUserID
	added := make([]auth.Account, 0, len(bundle.Accounts))
	updated := make([]auth.Account, 0, len(bundle.Accounts))
	for _, exported := range bundle.Accounts {
		account := auth.Account{UserID: exported.UserID, Username: exported.Username, RefreshToken: exported.RefreshToken}
		if store.Has(account.UserID) {
			updated = append(updated, account)
		} else {
			added = append(added, account)
		}
		store.Upsert(account)
	}
	if existingDefault == 0 {
		store.DefaultUserID = bundle.DefaultUserID
	}
	if err := auth.SaveAuthStore(authPath, store); err != nil {
		return nil, localSnapshotError(OperationRestoreAuthBundle, markLocalState(localStateStageAuth, err))
	}
	result := &AuthRestoreResult{
		DefaultUserID: store.DefaultUserID,
		Added:         make([]Account, len(added)),
		Updated:       make([]Account, len(updated)),
	}
	for i, account := range added {
		result.Added[i] = publicAccount(account, store.DefaultUserID)
	}
	for i, account := range updated {
		result.Updated[i] = publicAccount(account, store.DefaultUserID)
	}
	return result, nil
}

func validateAuthExportBundleHeader(bundle *AuthExportBundle) error {
	if bundle == nil || bundle.Schema != AuthExportBundleSchema {
		return errors.New("auth export bundle schema is unsupported")
	}
	if bundle.Version != AuthExportBundleVersion {
		return errors.New("auth export bundle version is unsupported")
	}
	return nil
}

func validateAuthExportBundle(bundle *AuthExportBundle) error {
	if err := validateAuthExportBundleHeader(bundle); err != nil {
		return err
	}
	if len(bundle.Accounts) == 0 {
		return errors.New("auth export bundle accounts are required")
	}
	seen := make(map[int64]struct{}, len(bundle.Accounts))
	for _, account := range bundle.Accounts {
		if account.UserID <= 0 {
			return errors.New("auth export account user id must be positive")
		}
		if _, exists := seen[account.UserID]; exists {
			return errors.New("auth export account user ids must be unique")
		}
		seen[account.UserID] = struct{}{}
		if strings.TrimSpace(account.RefreshToken) == "" {
			return errors.New("auth export account refresh token is required")
		}
	}
	if bundle.DefaultUserID <= 0 {
		return errors.New("auth export default user id must be positive")
	}
	if _, exists := seen[bundle.DefaultUserID]; !exists {
		return errors.New("auth export default user id must reference an account")
	}
	return nil
}
