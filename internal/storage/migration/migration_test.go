package migration_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/account/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	database "github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/migration"
)

type testFiles struct{ path string }

func (f testFiles) Path() (string, error)                { return f.path, nil }
func (f testFiles) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
func (f testFiles) WritePrivateFile(path string, body []byte) error {
	return os.WriteFile(path, body, localstate.PrivateFileMode)
}
func (f testFiles) EnsurePrivateFile(path string, body []byte) error {
	return os.WriteFile(path, body, localstate.PrivateFileMode)
}

func TestLegacyAccountPoolIsIdempotentAndRetriesCleanup(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte("[account_pool]\naccounts = [1]\nstrategy = \"random\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SavePixivCredential(context.Background(), accountpixiv.New(1, "user", []byte("token"))); err != nil {
		t.Fatal(err)
	}
	removeErr := errors.New("cleanup unavailable")
	if err := migration.LegacyAccountPool(context.Background(), db, testFiles{path: configPath}, func(string) error { return removeErr }); !errors.Is(err, removeErr) {
		t.Fatalf("first migration error = %v", err)
	}
	account, err := db.GetPixiv(context.Background(), 1)
	if err != nil || !account.Schedulable {
		t.Fatalf("DB mapping after cleanup failure = %+v, err=%v", account, err)
	}
	if err := migration.LegacyAccountPool(context.Background(), db, testFiles{path: configPath}, func(path string) error {
		return config.RemoveLegacyAccountPoolAccountsWithFileStore(path, testFiles{path: configPath})
	}); err != nil {
		t.Fatal(err)
	}
	if err := migration.LegacyAccountPool(context.Background(), db, testFiles{path: configPath}, nil); err != nil {
		t.Fatal(err)
	}
	state, err := config.LoadSnapshotAtWithFileStore(configPath, testFiles{path: configPath})
	if err != nil {
		t.Fatal(err)
	}
	if _, present, err := state.LegacyAccountPoolUIDs(); err != nil || present {
		t.Fatalf("legacy config after migration present=%v err=%v", present, err)
	}
}
