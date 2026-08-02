package authdb

import (
	"context"
	"errors"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestPixivUpsertListGetRemove(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	a := PixivAccount{UserID: 10, Username: "a", RefreshToken: []byte("t1"), CredentialRevision: 1}
	b := PixivAccount{UserID: 20, Username: "b", RefreshToken: []byte("t2"), CredentialRevision: 1}
	if err := db.UpsertPixiv(ctx, a); err != nil {
		t.Fatalf("upsert a: %v", err)
	}
	if err := db.UpsertPixiv(ctx, b); err != nil {
		t.Fatalf("upsert b: %v", err)
	}
	accounts, err := db.ListPixiv(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(accounts) != 2 || accounts[0].UserID != 10 || accounts[1].UserID != 20 {
		t.Fatalf("accounts = %+v", accounts)
	}
	if accounts[0].SortOrder != 1 || accounts[1].SortOrder != 2 {
		t.Fatalf("sort orders = %d, %d", accounts[0].SortOrder, accounts[1].SortOrder)
	}
	got, err := db.GetPixiv(ctx, 20)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got.RefreshToken) != "t2" {
		t.Fatalf("token = %q", got.RefreshToken)
	}
	if err := db.RemovePixiv(ctx, 10); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := db.GetPixiv(ctx, 10); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	// 重新插入不应复用被删除账号的 sort_order。
	if err := db.UpsertPixiv(ctx, PixivAccount{UserID: 30, Username: "c", RefreshToken: []byte("t3"), CredentialRevision: 1}); err != nil {
		t.Fatalf("upsert c: %v", err)
	}
	accounts, _ = db.ListPixiv(ctx)
	if accounts[1].SortOrder != 3 {
		t.Fatalf("c sort_order = %d, want 3 (no renumbering)", accounts[1].SortOrder)
	}
}

func TestPixivUpsertPreservesSortOrderOnReimport(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if err := db.UpsertPixiv(ctx, PixivAccount{UserID: 1, Username: "a", RefreshToken: []byte("t"), CredentialRevision: 1}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := db.UpsertPixiv(ctx, PixivAccount{UserID: 2, Username: "b", RefreshToken: []byte("t"), CredentialRevision: 1}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// 重新导入 user 1：保留原 sort_order。
	if err := db.UpsertPixiv(ctx, PixivAccount{UserID: 1, Username: "a2", RefreshToken: []byte("new"), CredentialRevision: 2}); err != nil {
		t.Fatalf("reimport: %v", err)
	}
	accounts, _ := db.ListPixiv(ctx)
	if accounts[0].UserID != 1 || accounts[0].SortOrder != 1 || string(accounts[0].RefreshToken) != "new" || accounts[0].CredentialRevision != 2 {
		t.Fatalf("reimported = %+v", accounts[0])
	}
}

func TestPixivRotateIncrementsRevision(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if err := db.UpsertPixiv(ctx, PixivAccount{UserID: 5, Username: "x", RefreshToken: []byte("old"), CredentialRevision: 1}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := db.RotatePixivCredentials(ctx, 5, []byte("rotated")); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	account, _ := db.GetPixiv(ctx, 5)
	if account.CredentialRevision != 2 || string(account.RefreshToken) != "rotated" {
		t.Fatalf("after rotate = %+v", account)
	}
}

func TestPixivPoolSelection(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	for _, id := range []int64{1, 2, 3} {
		if err := db.UpsertPixiv(ctx, PixivAccount{UserID: id, Username: "u", RefreshToken: []byte("t"), CredentialRevision: 1}); err != nil {
			t.Fatalf("upsert %d: %v", id, err)
		}
	}
	first, err := db.SelectPooledPixiv(ctx, 1000, nil)
	if err != nil {
		t.Fatalf("select first: %v", err)
	}
	if first.UserID != 1 {
		t.Fatalf("first selected = %d, want 1", first.UserID)
	}
	second, err := db.SelectPooledPixiv(ctx, 1001, nil)
	if err != nil {
		t.Fatalf("select second: %v", err)
	}
	if second.UserID != 2 {
		t.Fatalf("second selected = %d, want 2 (rotation)", second.UserID)
	}
	// 冻结账号 1 后，选择应跳过它。
	if err := db.FreezePooledPixiv(ctx, 1, 2000); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	third, err := db.SelectPooledPixiv(ctx, 1002, nil)
	if err != nil {
		t.Fatalf("select third: %v", err)
	}
	if third.UserID == 1 {
		t.Fatalf("frozen account selected")
	}
}

func TestPixivPoolFrozenExpiryCleared(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if err := db.UpsertPixiv(ctx, PixivAccount{UserID: 1, Username: "u", RefreshToken: []byte("t"), CredentialRevision: 1}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := db.UpsertPixiv(ctx, PixivAccount{UserID: 2, Username: "u", RefreshToken: []byte("t"), CredentialRevision: 1}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// 冻结账号 2 到过期时间；选择时应视为未冻结并清理。
	if err := db.FreezePooledPixiv(ctx, 2, 500); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	selected, err := db.SelectPooledPixiv(ctx, 1000, nil)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	account, _ := db.GetPixiv(ctx, 2)
	if account.PoolFrozenUntil != nil {
		t.Fatalf("expired freeze not cleared: %+v", account)
	}
	_ = selected
}

func TestPixivPoolAllowedFilter(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	for _, id := range []int64{1, 2, 3} {
		if err := db.UpsertPixiv(ctx, PixivAccount{UserID: id, Username: "u", RefreshToken: []byte("t"), CredentialRevision: 1}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	selected, err := db.SelectPooledPixiv(ctx, 1000, []int64{2, 3})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if selected.UserID == 1 {
		t.Fatalf("selected account outside allowed filter")
	}
}

func TestFanboxUpsertListRotateRemove(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	account := FanboxAccount{UserID: 7, DisplayName: "creator", CreatorID: "cid", SessionID: []byte("sess"), CredentialRevision: 1, ValidatedAt: 100}
	if err := db.UpsertFanbox(ctx, account); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := db.RotateFanboxSession(ctx, 7, []byte("new-sess"), 200); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	got, err := db.GetFanbox(ctx, 7)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got.SessionID) != "new-sess" || got.CredentialRevision != 2 {
		t.Fatalf("got = %+v", got)
	}
	list, err := db.ListFanbox(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %+v err=%v", list, err)
	}
	if err := db.RemoveFanbox(ctx, 7); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := db.GetFanbox(ctx, 7); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
