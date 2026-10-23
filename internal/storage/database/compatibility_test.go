package database_test

import (
	"context"
	"errors"
	"testing"

	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
)

// 零值和 NULL 不能被 GORM 的默认值处理改写。
func TestPixivCredentialAndMetadataPreserveZeroAndNull(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	premium := false
	checkedAt := int64(0)
	account := pixivAccount(1, "", "token")
	account.SortOrder = 7
	account.PremiumStatus = &premium
	account.PremiumCheckedAt = &checkedAt
	if err := db.SavePixivCredential(ctx, account); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetPixiv(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.SortOrder != 7 || got.Username != "" || got.PremiumStatus == nil || *got.PremiumStatus || got.PremiumCheckedAt == nil || *got.PremiumCheckedAt != 0 {
		t.Fatalf("saved zero values = %+v", got)
	}
	if err := db.UpdatePixivMetadata(ctx, 1, "user", nil, nil); err != nil {
		t.Fatal(err)
	}
	got, err = db.GetPixiv(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.PremiumStatus != nil || got.PremiumCheckedAt != nil {
		t.Fatalf("NULL metadata = %+v", got)
	}
}

func TestSelectPixivChooserFailureRollsBackMarker(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if err := db.SavePixivCredential(ctx, pixivAccount(1, "u", "token")); err != nil {
		t.Fatal(err)
	}
	if err := db.Freeze(ctx, 1, 100); err != nil {
		t.Fatal(err)
	}
	before, err := db.GetPixiv(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SelectPixiv(ctx, 100, nil, func(accountpixiv.PoolSnapshot) (int64, error) {
		return 1, errors.New("chooser failed")
	}); err == nil {
		t.Fatal("chooser failure was reported as success")
	}
	after, err := db.GetPixiv(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if after.PoolLastSelected || after.UpdatedAt != before.UpdatedAt ||
		after.PoolFrozenUntil == nil || *after.PoolFrozenUntil != 100 {
		t.Fatalf("rollback left transaction changes: before=%+v after=%+v", before, after)
	}
}

func TestPixivSchedulableWritesPersistFalse(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if err := db.SavePixivCredential(ctx, pixivAccount(1, "u", "token")); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPixivSchedulable(ctx, []int64{1}, false); err != nil {
		t.Fatal(err)
	}
	account, err := db.GetPixiv(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if account.Schedulable {
		t.Fatal("false schedulable write was lost")
	}
	if err := db.SetAllPixivSchedulable(ctx, true); err != nil {
		t.Fatal(err)
	}
	account, err = db.GetPixiv(ctx, 1)
	if err != nil || !account.Schedulable {
		t.Fatalf("true schedulable write did not persist: %+v %v", account, err)
	}
}

func TestListPixivPoolStatusClearsExpiredFreezeInTransaction(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if err := db.SavePixivCredential(ctx, pixivAccount(1, "u", "token")); err != nil {
		t.Fatal(err)
	}
	if err := db.Freeze(ctx, 1, 100); err != nil {
		t.Fatal(err)
	}
	status, err := db.ListPixivPoolStatus(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Accounts) != 1 || status.Accounts[0].PoolFrozenUntil != nil || status.EarliestFrozenUntil != nil {
		t.Fatalf("expired freeze = %+v", status)
	}
	got, err := db.GetPixiv(ctx, 1)
	if err != nil || got.PoolFrozenUntil != nil {
		t.Fatalf("expired freeze was not committed: %+v %v", got, err)
	}
}
