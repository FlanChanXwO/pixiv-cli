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
	if err := db.SavePixivCredential(ctx, a); err != nil {
		t.Fatalf("upsert a: %v", err)
	}
	if err := db.SavePixivCredential(ctx, b); err != nil {
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
	if err := db.SavePixivCredential(ctx, PixivAccount{UserID: 30, Username: "c", RefreshToken: []byte("t3"), CredentialRevision: 1}); err != nil {
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
	if err := db.SavePixivCredential(ctx, PixivAccount{UserID: 1, Username: "a", RefreshToken: []byte("t"), CredentialRevision: 1}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := db.SavePixivCredential(ctx, PixivAccount{UserID: 2, Username: "b", RefreshToken: []byte("t"), CredentialRevision: 1}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// 重新导入 user 1：保留原 sort_order。
	if err := db.SavePixivCredential(ctx, PixivAccount{UserID: 1, Username: "a2", RefreshToken: []byte("new"), CredentialRevision: 2}); err != nil {
		t.Fatalf("reimport: %v", err)
	}
	accounts, _ := db.ListPixiv(ctx)
	if accounts[0].UserID != 1 || accounts[0].SortOrder != 1 || string(accounts[0].RefreshToken) != "new" || accounts[0].CredentialRevision != 2 {
		t.Fatalf("reimported = %+v", accounts[0])
	}
}

func TestPixivCredentialReplacementIncrementsRevisionAndMetadataDoesNot(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if err := db.SavePixivCredential(ctx, PixivAccount{UserID: 1, Username: "a", RefreshToken: []byte("first"), CredentialRevision: 99}); err != nil {
		t.Fatalf("initial save: %v", err)
	}
	account, err := db.GetPixiv(ctx, 1)
	if err != nil {
		t.Fatalf("get initial: %v", err)
	}
	if account.CredentialRevision != 1 {
		t.Fatalf("initial revision = %d, want 1", account.CredentialRevision)
	}

	if err := db.SavePixivCredential(ctx, PixivAccount{UserID: 1, Username: "a2", RefreshToken: []byte("replacement"), CredentialRevision: 1}); err != nil {
		t.Fatalf("replacement save: %v", err)
	}
	account, err = db.GetPixiv(ctx, 1)
	if err != nil {
		t.Fatalf("get replacement: %v", err)
	}
	if account.CredentialRevision != 2 || string(account.RefreshToken) != "replacement" {
		t.Fatalf("replacement account = %+v, want revision 2 and replacement credential", account)
	}

	premium := true
	checkedAt := int64(123)
	if err := db.UpdatePixivMetadata(ctx, 1, "a2", &premium, &checkedAt); err != nil {
		t.Fatalf("metadata update: %v", err)
	}
	account, err = db.GetPixiv(ctx, 1)
	if err != nil {
		t.Fatalf("get metadata: %v", err)
	}
	if account.CredentialRevision != 2 || string(account.RefreshToken) != "replacement" || account.PremiumStatus == nil || !*account.PremiumStatus || account.PremiumCheckedAt == nil || *account.PremiumCheckedAt != checkedAt {
		t.Fatalf("metadata update changed credential state: %+v", account)
	}
}

func TestPixivRotateIncrementsRevision(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if err := db.SavePixivCredential(ctx, PixivAccount{UserID: 5, Username: "x", RefreshToken: []byte("old"), CredentialRevision: 1}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := db.RotatePixivCredentials(ctx, 5, 1, []byte("rotated")); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	account, _ := db.GetPixiv(ctx, 5)
	if account.CredentialRevision != 2 || string(account.RefreshToken) != "rotated" {
		t.Fatalf("after rotate = %+v", account)
	}
}

func TestPixivRotationUsesExpectedRevisionAndReportsConflict(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if err := db.SavePixivCredential(ctx, PixivAccount{UserID: 5, Username: "x", RefreshToken: []byte("old"), CredentialRevision: 1}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := db.RotatePixivCredentials(ctx, 5, 1, []byte("first-rotation")); err != nil {
		t.Fatalf("first rotation: %v", err)
	}
	if err := db.RotatePixivCredentials(ctx, 5, 1, []byte("stale-rotation")); !errors.Is(err, ErrCredentialConflict) {
		t.Fatalf("stale rotation error = %v, want ErrCredentialConflict", err)
	}
	account, err := db.GetPixiv(ctx, 5)
	if err != nil {
		t.Fatalf("get after conflict: %v", err)
	}
	if account.CredentialRevision != 2 || string(account.RefreshToken) != "first-rotation" {
		t.Fatalf("stale rotation changed account: %+v", account)
	}
	if err := db.RotatePixivCredentials(ctx, 999, 1, []byte("missing")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing rotation error = %v, want ErrNotFound", err)
	}
}

func TestPixivPoolSelection(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	for _, id := range []int64{1, 2, 3} {
		if err := db.SavePixivCredential(ctx, PixivAccount{UserID: id, Username: "u", RefreshToken: []byte("t"), CredentialRevision: 1}); err != nil {
			t.Fatalf("upsert %d: %v", id, err)
		}
	}
	first, err := db.SelectPooledPixiv(ctx, 1000, PoolStrategyRoundRobin, nil)
	if err != nil {
		t.Fatalf("select first: %v", err)
	}
	if first.UserID != 1 {
		t.Fatalf("first selected = %d, want 1", first.UserID)
	}
	second, err := db.SelectPooledPixiv(ctx, 1001, PoolStrategyRoundRobin, nil)
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
	third, err := db.SelectPooledPixiv(ctx, 1002, PoolStrategyRoundRobin, nil)
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
	if err := db.SavePixivCredential(ctx, PixivAccount{UserID: 1, Username: "u", RefreshToken: []byte("t"), CredentialRevision: 1}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := db.SavePixivCredential(ctx, PixivAccount{UserID: 2, Username: "u", RefreshToken: []byte("t"), CredentialRevision: 1}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// 冻结账号 2 到过期时间；选择时应视为未冻结并清理。
	if err := db.FreezePooledPixiv(ctx, 2, 500); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	selected, err := db.SelectPooledPixiv(ctx, 1000, PoolStrategyRoundRobin, nil)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	account, _ := db.GetPixiv(ctx, 2)
	if account.PoolFrozenUntil != nil {
		t.Fatalf("expired freeze not cleared: %+v", account)
	}
	_ = selected
}

func TestPixivPoolSchedulableFilter(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	for _, id := range []int64{1, 2, 3} {
		if err := db.SavePixivCredential(ctx, PixivAccount{UserID: id, Username: "u", RefreshToken: []byte("t"), CredentialRevision: 1}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	if err := db.SetPixivSchedulable(ctx, []int64{1}, false); err != nil {
		t.Fatalf("disable account 1: %v", err)
	}
	selected, err := db.SelectPooledPixiv(ctx, 1000, PoolStrategyRoundRobin, nil)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if selected.UserID == 1 {
		t.Fatalf("selected disabled account")
	}
}

func TestPixivPoolRoundRobinDoesNotStarveAccounts(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	for _, id := range []int64{1, 2, 3, 4} {
		if err := db.SavePixivCredential(ctx, PixivAccount{UserID: id, Username: "u", RefreshToken: []byte("t")}); err != nil {
			t.Fatalf("save %d: %v", id, err)
		}
	}
	var got []int64
	for i := int64(0); i < 8; i++ {
		account, err := db.SelectPooledPixiv(ctx, 1000+i, PoolStrategyRoundRobin, nil)
		if err != nil {
			t.Fatalf("select %d: %v", i, err)
		}
		got = append(got, account.UserID)
	}
	want := []int64{1, 2, 3, 4, 1, 2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("selection = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("selection = %v, want %v", got, want)
		}
	}
}

func TestPixivPoolMarkerCanBeFrozenDisabledOrDeleted(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	for _, id := range []int64{1, 2, 3} {
		if err := db.SavePixivCredential(ctx, PixivAccount{UserID: id, Username: "u", RefreshToken: []byte("t")}); err != nil {
			t.Fatalf("save %d: %v", id, err)
		}
	}
	selected, err := db.SelectPooledPixiv(ctx, 100, PoolStrategyRoundRobin, nil)
	if err != nil || selected.UserID != 1 {
		t.Fatalf("initial select = %+v err=%v", selected, err)
	}
	if err := db.FreezePooledPixiv(ctx, 1, 200); err != nil {
		t.Fatalf("freeze marker: %v", err)
	}
	selected, err = db.SelectPooledPixiv(ctx, 101, PoolStrategyRoundRobin, nil)
	if err != nil || selected.UserID != 2 {
		t.Fatalf("frozen marker select = %+v err=%v", selected, err)
	}
	if err := db.SetPixivSchedulable(ctx, []int64{2}, false); err != nil {
		t.Fatalf("disable marker: %v", err)
	}
	selected, err = db.SelectPooledPixiv(ctx, 102, PoolStrategyRoundRobin, nil)
	if err != nil || selected.UserID != 3 {
		t.Fatalf("disabled marker select = %+v err=%v", selected, err)
	}
	if err := db.RemovePixiv(ctx, 3); err != nil {
		t.Fatalf("remove marker: %v", err)
	}
	selected, err = db.SelectPooledPixiv(ctx, 201, PoolStrategyRoundRobin, nil)
	if err != nil || selected.UserID != 1 {
		t.Fatalf("deleted marker wrap = %+v err=%v", selected, err)
	}
}

func TestPixivPoolBatchUpdateIsAtomicForUnknownUID(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	for _, id := range []int64{1, 2} {
		if err := db.SavePixivCredential(ctx, PixivAccount{UserID: id, Username: "u", RefreshToken: []byte("t")}); err != nil {
			t.Fatalf("save %d: %v", id, err)
		}
	}
	if err := db.SetPixivSchedulable(ctx, []int64{1, 999}, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("batch error = %v, want ErrNotFound", err)
	}
	first, _ := db.GetPixiv(ctx, 1)
	second, _ := db.GetPixiv(ctx, 2)
	if !first.Schedulable || !second.Schedulable {
		t.Fatalf("failed batch partially changed rows: first=%+v second=%+v", first, second)
	}
}

func TestPixivPoolSelectionDistinguishesNoAccountAndAllFrozen(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	_, err := db.SelectPooledPixiv(ctx, 100, PoolStrategyRoundRobin, nil)
	var selectionErr *PoolSelectionError
	if !errors.As(err, &selectionErr) || selectionErr.Kind != PoolSelectionNoLocalAccount {
		t.Fatalf("no account error = %v", err)
	}
	if err := db.SavePixivCredential(ctx, PixivAccount{UserID: 1, Username: "u", RefreshToken: []byte("t")}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := db.FreezePooledPixiv(ctx, 1, 250); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	_, err = db.SelectPooledPixiv(ctx, 100, PoolStrategyRoundRobin, nil)
	if !errors.As(err, &selectionErr) || selectionErr.Kind != PoolSelectionAllFrozen || selectionErr.EarliestFrozenUntil == nil || *selectionErr.EarliestFrozenUntil != 250 {
		t.Fatalf("all frozen error = %v", err)
	}
}

func TestPixivPoolRandomOnlySelectsEligibleAccount(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	for _, id := range []int64{1, 2, 3} {
		if err := db.SavePixivCredential(ctx, PixivAccount{UserID: id, Username: "u", RefreshToken: []byte("t")}); err != nil {
			t.Fatalf("save %d: %v", id, err)
		}
	}
	if err := db.SetPixivSchedulable(ctx, []int64{1}, false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	db.poolRandom = func(size int) (int, error) { return size - 1, nil }
	selected, err := db.SelectPooledPixiv(ctx, 100, PoolStrategyRandom, nil)
	if err != nil {
		t.Fatalf("random select: %v", err)
	}
	if selected.UserID != 3 {
		t.Fatalf("random selected = %d, want eligible last account 3", selected.UserID)
	}
}

func TestFanboxUpsertListRotateRemove(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	account := FanboxAccount{UserID: 7, DisplayName: "creator", CreatorID: "cid", SessionID: []byte("sess"), CredentialRevision: 1, ValidatedAt: 100}
	if err := db.SaveFanboxCredential(ctx, account); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := db.RotateFanboxSession(ctx, 7, 1, []byte("new-sess"), 200); err != nil {
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

func TestFanboxRotationValidatesTimestampAndExpectedRevision(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if err := db.SaveFanboxCredential(ctx, FanboxAccount{UserID: 7, DisplayName: "creator", SessionID: []byte("sess"), CredentialRevision: 1, ValidatedAt: 100}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := db.RotateFanboxSession(ctx, 7, 1, []byte("new-sess"), 0); err == nil {
		t.Fatal("zero validated_at rotation should fail")
	}
	if err := db.RotateFanboxSession(ctx, 7, 1, []byte("new-sess"), 200); err != nil {
		t.Fatalf("valid rotation: %v", err)
	}
	if err := db.RotateFanboxSession(ctx, 7, 1, []byte("stale-sess"), 300); !errors.Is(err, ErrCredentialConflict) {
		t.Fatalf("stale rotation error = %v, want ErrCredentialConflict", err)
	}
	account, err := db.GetFanbox(ctx, 7)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if account.CredentialRevision != 2 || string(account.SessionID) != "new-sess" || account.ValidatedAt != 200 {
		t.Fatalf("stale rotation changed account: %+v", account)
	}
}

func TestFanboxEmptyCreatorIDRoundTrips(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if err := db.SaveFanboxCredential(ctx, FanboxAccount{UserID: 8, DisplayName: "creator", SessionID: []byte("sess"), CredentialRevision: 1, ValidatedAt: 100}); err != nil {
		t.Fatalf("save: %v", err)
	}
	account, err := db.GetFanbox(ctx, 8)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if account.CreatorID != "" {
		t.Fatalf("CreatorID = %q, want empty", account.CreatorID)
	}
}
