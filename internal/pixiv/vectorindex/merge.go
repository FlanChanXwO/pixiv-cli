package vectorindex

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/filelock"
)

// MergeReport 保留已合并条数和原样隔离的坏行条数。
type MergeReport struct {
	Merged         int `json:"merged"`
	SkippedRecords int `json:"skipped_records"`
}

// Merge 先恢复上次的合并文件，再换入当前收件箱；调用方须持有同步锁。
// 持久化失败保留合并文件供恢复，已隔离坏行在恢复时可能再次保留。
func (s *Store) Merge(ctx context.Context) (report MergeReport, err error) {
	merging := filepath.Join(s.directory, mergingName)
	if err = s.mergeFile(ctx, merging, &report); err != nil {
		return report, err
	}
	err = filelock.WithPrivateLock(ctx, filepath.Join(filepath.Dir(s.directory), inboxLockName), func() error {
		err := os.Rename(filepath.Join(s.directory, inboxName), merging)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	})
	if err != nil {
		return report, err
	}
	err = s.mergeFile(ctx, merging, &report)
	return report, err
}

func (s *Store) mergeFile(ctx context.Context, path string, report *MergeReport) (err error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, file.Close())
		}
	}()
	reader := bufio.NewReader(file)
	for line := 1; ; line++ {
		body, readErr := reader.ReadBytes('\n')
		if len(body) > 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
			var value entry
			if err := json.Unmarshal(body, &value); err != nil || !validEntry(value) {
				if err := appendPrivate(filepath.Join(s.directory, rejectedName), body); err != nil {
					return err
				}
				report.SkippedRecords++
				slog.WarnContext(ctx, "Invalid vector inbox record preserved", "line", line, "reason", "invalid_record")
			} else {
				if err := s.put(ctx, value); err != nil {
					return err
				}
				report.Merged++
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	closed = true
	if err = file.Close(); err != nil {
		return err
	}
	return os.Remove(path)
}

func validEntry(value entry) bool {
	if value.RecordedAt.IsZero() || len(value.Pages) == 0 {
		return false
	}
	// Record 的 JSON 入口已校验身份；零值表示缺少 record 字段。
	body, err := json.Marshal(value.Record)
	if err != nil || string(body) == "null" || string(body) == "{}" {
		return false
	}
	if value.Record.Type() != "artwork" {
		return false
	}
	for _, raw := range value.Pages {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host != "i.pximg.net" {
			return false
		}
	}
	return true
}

func (s *Store) put(ctx context.Context, value entry) (err error) {
	body, err := json.Marshal(value.Record)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer func() {
		rollback := tx.Rollback()
		if !errors.Is(rollback, sql.ErrTxDone) {
			err = errors.Join(err, rollback)
		}
	}()
	id := value.Record.ID()
	var recorded string
	err = tx.QueryRowContext(ctx, `SELECT recorded_at FROM artwork WHERE id=?`, id).Scan(&recorded)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		old, err := time.Parse(time.RFC3339Nano, recorded)
		if err != nil {
			return fmt.Errorf("invalid vector record timestamp: %w", err)
		}
		if !value.RecordedAt.After(old) {
			return nil
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO artwork VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET record=excluded.record, recorded_at=excluded.recorded_at`, id, body, value.RecordedAt.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM page WHERE artwork_id=? AND page_index>=?`, id, len(value.Pages)); err != nil {
		return err
	}
	for i, raw := range value.Pages {
		if _, err = tx.ExecContext(ctx, `DELETE FROM embedding WHERE artwork_id=? AND page_index=? AND EXISTS(SELECT 1 FROM page WHERE artwork_id=? AND page_index=? AND url<>?)`, id, i, id, i, raw); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO page(artwork_id,page_index,url) VALUES(?,?,?) ON CONFLICT(artwork_id,page_index) DO UPDATE SET url=excluded.url, unavailable=CASE WHEN page.url=excluded.url THEN page.unavailable ELSE '' END`, id, i, raw); err != nil {
			return err
		}
	}
	return tx.Commit()
}
