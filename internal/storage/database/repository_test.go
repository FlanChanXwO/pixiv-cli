package database_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	accountfanbox "github.com/FlanChanXwO/pixiv-cli/internal/account/fanbox"
	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/account/pixiv"
	sessionpixiv "github.com/FlanChanXwO/pixiv-cli/internal/session/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	database "github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
)

func openTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func pixivAccount(userID int64, username, token string) accountpixiv.Account {
	return accountpixiv.New(userID, username, []byte(token))
}

func fanboxAccount(userID int64, displayName, creatorID, session string, validatedAt int64) accountfanbox.Account {
	account := accountfanbox.New(userID, displayName, creatorID, []byte(session))
	account.ValidatedAt = validatedAt
	return account
}

func choose(strategy config.AccountPoolStrategy, random func(int) (int, error)) accountpixiv.Chooser {
	return func(snapshot accountpixiv.PoolSnapshot) (int64, error) {
		return sessionpixiv.Choose(snapshot, strategy, random)
	}
}

func TestPixivCredentialAndAccountState(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	token := []byte("t1")
	if err := db.SavePixivCredential(ctx, accountpixiv.New(10, "a", token)); err != nil {
		t.Fatalf("save: %v", err)
	}
	token[0] = 'x'
	got, err := db.GetPixiv(ctx, 10)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got.RefreshTokenCopy()) != "t1" {
		t.Fatalf("stored token changed through input alias")
	}
	returned := got.RefreshTokenCopy()
	returned[0] = 'y'
	gotAgain, err := db.GetPixiv(ctx, 10)
	if err != nil || string(gotAgain.RefreshTokenCopy()) != "t1" {
		t.Fatalf("stored token changed through output alias: %v", err)
	}

	if err := db.SavePixivCredential(ctx, pixivAccount(20, "b", "t2")); err != nil {
		t.Fatalf("save second: %v", err)
	}
	accounts, err := db.ListPixiv(ctx)
	if err != nil || len(accounts) != 2 || accounts[0].SortOrder != 1 || accounts[1].SortOrder != 2 {
		t.Fatalf("list = %+v, err=%v", accounts, err)
	}
	if err := db.RemovePixiv(ctx, 10); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := db.GetPixiv(ctx, 10); !errors.Is(err, accountpixiv.ErrNotFound) {
		t.Fatalf("get removed error = %v", err)
	}
	if err := db.SavePixivCredential(ctx, pixivAccount(30, "c", "t3")); err != nil {
		t.Fatalf("save after remove: %v", err)
	}
	accounts, _ = db.ListPixiv(ctx)
	if accounts[1].SortOrder != 3 {
		t.Fatalf("new sort order = %d, want 3", accounts[1].SortOrder)
	}
}

func TestPixivReimportMetadataAndRotationCAS(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if err := db.SavePixivCredential(ctx, pixivAccount(1, "a", "first")); err != nil {
		t.Fatal(err)
	}
	if err := db.SavePixivCredential(ctx, pixivAccount(1, "a2", "replacement")); err != nil {
		t.Fatal(err)
	}
	account, err := db.GetPixiv(ctx, 1)
	if err != nil || account.CredentialRevision != 2 || string(account.RefreshTokenCopy()) != "replacement" {
		t.Fatalf("reimport = %+v, err=%v", account, err)
	}
	premium := true
	checkedAt := int64(123)
	if err := db.UpdatePixivMetadata(ctx, 1, "a2", &premium, &checkedAt); err != nil {
		t.Fatal(err)
	}
	account, _ = db.GetPixiv(ctx, 1)
	if account.CredentialRevision != 2 || string(account.RefreshTokenCopy()) != "replacement" || account.PremiumStatus == nil || !*account.PremiumStatus {
		t.Fatalf("metadata changed credential state: %+v", account)
	}
	if err := db.RotatePixivCredentials(ctx, 1, 2, []byte("rotated")); err != nil {
		t.Fatal(err)
	}
	if err := db.RotatePixivCredentials(ctx, 1, 2, []byte("stale")); !errors.Is(err, accountpixiv.ErrCredentialConflict) {
		t.Fatalf("stale rotation error = %v", err)
	}
	if err := db.RotatePixivCredentials(ctx, 999, 1, []byte("missing")); !errors.Is(err, accountpixiv.ErrNotFound) {
		t.Fatalf("missing rotation error = %v", err)
	}
}

func TestPixivPoolTransactionChooserFreezeAndValidation(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	for _, id := range []int64{1, 2, 3} {
		if err := db.SavePixivCredential(ctx, pixivAccount(id, "u", "token")); err != nil {
			t.Fatal(err)
		}
	}
	first, err := db.SelectPixiv(ctx, 100, nil, choose(config.AccountPoolStrategyRoundRobin, nil))
	if err != nil || first.UserID != 1 {
		t.Fatalf("first = %+v, err=%v", first, err)
	}
	second, err := db.SelectPixiv(ctx, 101, nil, choose(config.AccountPoolStrategyRoundRobin, nil))
	if err != nil || second.UserID != 2 {
		t.Fatalf("second = %+v, err=%v", second, err)
	}
	if err := db.FreezePooledPixiv(ctx, 2, 200); err != nil {
		t.Fatal(err)
	}
	third, err := db.SelectPixiv(ctx, 102, nil, choose(config.AccountPoolStrategyRoundRobin, nil))
	if err != nil || third.UserID != 3 {
		t.Fatalf("frozen marker selection = %+v, err=%v", third, err)
	}
	if err := db.FreezePooledPixiv(ctx, 1, 300); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SelectPixiv(ctx, 150, nil, func(accountpixiv.PoolSnapshot) (int64, error) { return 999, nil }); err == nil {
		t.Fatal("chooser outside snapshot unexpectedly committed")
	}
	status, err := db.ListPixivPoolStatus(ctx, 150)
	if err != nil || len(status.Accounts) != 3 {
		t.Fatalf("status = %+v, err=%v", status, err)
	}
	if status.Accounts[0].PoolLastSelected || !status.Accounts[2].PoolLastSelected {
		t.Fatalf("invalid chooser changed marker: %+v", status.Accounts)
	}
}

func TestPixivPoolErrorsAndRandomChooser(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	_, err := db.SelectPixiv(ctx, 100, nil, choose(config.AccountPoolStrategyRoundRobin, nil))
	var selectionErr *accountpixiv.PoolSelectionError
	if !errors.As(err, &selectionErr) || selectionErr.Kind != accountpixiv.PoolSelectionNoLocalAccount {
		t.Fatalf("no account error = %v", err)
	}
	if err := db.SavePixivCredential(ctx, pixivAccount(1, "u", "token")); err != nil {
		t.Fatal(err)
	}
	if err := db.FreezePooledPixiv(ctx, 1, 250); err != nil {
		t.Fatal(err)
	}
	_, err = db.SelectPixiv(ctx, 100, nil, choose(config.AccountPoolStrategyRoundRobin, nil))
	if !errors.As(err, &selectionErr) || selectionErr.Kind != accountpixiv.PoolSelectionAllFrozen || selectionErr.EarliestFrozenUntil == nil || *selectionErr.EarliestFrozenUntil != 250 {
		t.Fatalf("all frozen error = %v", err)
	}

	db = openTestDB(t)
	for _, id := range []int64{1, 2, 3} {
		if err := db.SavePixivCredential(ctx, pixivAccount(id, "u", "token")); err != nil {
			t.Fatal(err)
		}
	}
	selected, err := db.SelectPixiv(ctx, 100, nil, choose(config.AccountPoolStrategyRandom, func(size int) (int, error) { return size - 1, nil }))
	if err != nil || selected.UserID != 3 {
		t.Fatalf("random selection = %+v, err=%v", selected, err)
	}
	if _, err := db.SelectPixiv(ctx, 100, []int64{1, 2, 3}, choose(config.AccountPoolStrategyRoundRobin, nil)); err == nil {
		t.Fatal("exhausted candidate set unexpectedly succeeded")
	}
}

func TestPixivRotationCASRace(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if err := db.SavePixivCredential(ctx, pixivAccount(1, "u", "token")); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var successes int
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := db.RotatePixivCredentials(ctx, 1, 1, []byte(fmt.Sprintf("rotation-%d", i))); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if successes != 1 {
		t.Fatalf("rotation successes = %d, want 1", successes)
	}
}

func TestFanboxCredentialAndRotationCAS(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if err := db.SaveFanboxCredential(ctx, fanboxAccount(7, "creator", "cid", "session", 100)); err != nil {
		t.Fatal(err)
	}
	if err := db.RotateFanboxSession(ctx, 7, 1, []byte("new-session"), 200); err != nil {
		t.Fatal(err)
	}
	if err := db.RotateFanboxSession(ctx, 7, 1, []byte("stale"), 300); !errors.Is(err, accountfanbox.ErrCredentialConflict) {
		t.Fatalf("stale rotation error = %v", err)
	}
	got, err := db.GetFanbox(ctx, 7)
	if err != nil || string(got.SessionIDCopy()) != "new-session" || got.CredentialRevision != 2 {
		t.Fatalf("fanbox account = %+v, err=%v", got, err)
	}
	if _, err := db.GetFanbox(ctx, 999); !errors.Is(err, accountfanbox.ErrNotFound) {
		t.Fatalf("missing fanbox error = %v", err)
	}
}
