package cli

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/account/pixiv"
	configapp "github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	"github.com/stretchr/testify/require"
)

// newTestResources 为需要真实 database-backed application service 的 CLI 测试
// 准备一次私有 graph；Run 结束和 t.Cleanup 的第二次关闭都必须保持幂等。
func newTestResources(t *testing.T) *runResources {
	t.Helper()
	resources := &runResources{}
	require.NoError(t, resources.prepare(commandResources{
		Database:     true,
		PixivAccount: true,
		PixivLogin:   true,
		PixivSDK:     true,
		Download:     true,
		Fanbox:       true,
	}))
	t.Cleanup(func() { require.NoError(t, resources.close()) })
	return resources
}

// testAuthStore 是旧 JSON fixture 的最小测试表示；写入时直接建立新的
// database 记录，避免测试继续依赖已删除的 auth.json 存储实现。
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
	db, err := database.Open(filepath.Dir(databasePath))
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
		stored := accountpixiv.New(account.UserID, account.Username, []byte(account.RefreshToken))
		stored.PremiumStatus = account.PremiumStatus
		stored.PremiumCheckedAt = checkedAt
		if err := db.SavePixivCredential(context.Background(), stored); err != nil {
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
