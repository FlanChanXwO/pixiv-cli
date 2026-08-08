package cli

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	configapp "github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/bootstrap"
	"github.com/FlanChanXwO/pixiv-cli/internal/persistence/authdb"
	"github.com/stretchr/testify/require"
)

// newTestRuntime 为需要真实 authdb-backed application service 的 CLI 测试提供
// 一个明确拥有者；Run 结束和 t.Cleanup 的第二次 Close 都由 Runtime 的幂等契约处理。
func newTestRuntime(t *testing.T) *bootstrap.Runtime {
	t.Helper()
	runtime, err := bootstrap.NewRuntime(bootstrap.RuntimeOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	return runtime
}

// testAuthStore 是旧 JSON fixture 的最小测试表示；写入时直接建立新的
// authdb 记录，避免测试继续依赖已删除的 auth.json 存储实现。
type testAuthStore struct {
	DefaultUserID int64
	Accounts      []testAuthAccount
}

type testAuthAccount struct {
	UserID                 int64
	Username               string
	RefreshToken           string
	PremiumStatus          *bool
	PremiumStatusCheckedAt *time.Time
}

func saveTestAuthStore(t *testing.T, databasePath string, store testAuthStore) error {
	t.Helper()
	db, err := authdb.Open(filepath.Dir(databasePath))
	if err != nil {
		return err
	}
	defer func() { require.NoError(t, db.Close()) }()
	for _, account := range store.Accounts {
		var checkedAt *int64
		if account.PremiumStatusCheckedAt != nil {
			value := account.PremiumStatusCheckedAt.UTC().Unix()
			checkedAt = &value
		}
		if err := db.SavePixivCredential(context.Background(), authdb.PixivAccount{
			UserID: account.UserID, Username: account.Username, RefreshToken: []byte(account.RefreshToken),
			PremiumStatus: account.PremiumStatus, PremiumCheckedAt: checkedAt,
		}); err != nil {
			return err
		}
	}
	if store.DefaultUserID > 0 {
		if err := configapp.SetPixivDefaultUserID(store.DefaultUserID); err != nil {
			return err
		}
	}
	return nil
}
