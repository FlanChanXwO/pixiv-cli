// Package migration owns repeatable migrations that coordinate two independent
// local stores. It never pretends that a database commit and a config rewrite
// form one cross-file transaction.
package migration

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
)

// LegacyAccountPoolVersion identifies the legacy config-to-database migration.
// The operation is idempotent by construction: DB state is rewritten from the
// validated snapshot, while config cleanup is retried until it succeeds.
const LegacyAccountPoolVersion = 1

// RemoveFunc is injected only to make cleanup failure/retry observable without
// changing the production file mechanism.
type RemoveFunc func(string) error

// LegacyAccountPool migrates account_pool.accounts into pixiv_account.schedulable
// and then removes the legacy config key. A successful DB write followed by a
// failed cleanup returns an error; the next invocation repeats the DB mapping
// and retries cleanup.
func LegacyAccountPool(ctx context.Context, db *database.DB, files config.FileStore, remove RemoveFunc) error {
	if ctx == nil {
		return errors.New("storage migration: context is nil")
	}
	if db == nil {
		return errors.New("storage migration: database is not configured")
	}
	if files == nil {
		return errors.New("storage migration: config file store is not configured")
	}
	path, err := files.Path()
	if err != nil {
		return fmt.Errorf("account pool migration: resolve config path: %w", err)
	}
	state, err := config.LoadSnapshotAtWithFileStore(path, files)
	if err != nil {
		return fmt.Errorf("account pool migration: read config: %w", err)
	}
	userIDs, present, err := state.LegacyAccountPoolUIDs()
	if err != nil {
		return fmt.Errorf("account pool migration: validate legacy accounts: %w", err)
	}
	if !present {
		return nil
	}
	if err := db.MigratePixivSchedulable(ctx, userIDs); err != nil {
		return fmt.Errorf("account pool migration: update database: %w", err)
	}
	if remove == nil {
		remove = func(path string) error {
			return config.RemoveLegacyAccountPoolAccountsWithFileStore(path, files)
		}
	}
	if err := remove(path); err != nil {
		return fmt.Errorf("account pool migration: remove legacy accounts: %w", err)
	}
	return nil
}
