package vectorindex

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

type bookmarkState struct {
	Cursor   sdk.Cursor `json:"cursor"`
	Complete bool       `json:"complete"`
}

func bookmarkKey(id int64, scope pixiv.Restrict) string {
	return fmt.Sprintf("bookmarks/%d/%s", id, scope)
}

func (s *Store) readBookmarkState(ctx context.Context, id int64, scope pixiv.Restrict) (bookmarkState, error) {
	var body string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM state WHERE key=?`, bookmarkKey(id, scope)).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return bookmarkState{}, nil
	}
	if err != nil {
		return bookmarkState{}, err
	}
	var state bookmarkState
	if err = json.Unmarshal([]byte(body), &state); err != nil {
		if sdk.ReasonOf(err) == sdk.InvalidArgument {
			slog.WarnContext(ctx, "Restarting bookmarks after invalid saved cursor")
			return bookmarkState{}, nil
		}
		return bookmarkState{}, fmt.Errorf("invalid bookmark sync state: %w", err)
	}
	return state, nil
}

func (s *Store) writeBookmarkState(ctx context.Context, id int64, scope pixiv.Restrict, state bookmarkState) error {
	body, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO state VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, bookmarkKey(id, scope), string(body))
	return err
}

func (s *Store) saveBookmarkBatch(ctx context.Context, id int64, scope pixiv.Restrict, items []*pixiv.Artwork, state bookmarkState, head bool) (added int, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if rollback := tx.Rollback(); !errors.Is(rollback, sql.ErrTxDone) {
			err = errors.Join(err, rollback)
		}
	}()
	for _, a := range items {
		result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO bookmark VALUES(?,?,?)`, id, string(scope), fmt.Sprint(a.ID))
		if err != nil {
			return 0, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		added += int(n)
	}
	if !head {
		body, err := json.Marshal(state)
		if err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO state VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, bookmarkKey(id, scope), string(body)); err != nil {
			return 0, err
		}
	}
	return added, tx.Commit()
}

// shortcut: 重新收藏的整批旧作品可能遮住后面的新收藏，需精确变更源时改用上游事件游标。
func (s *Store) syncBookmarks(ctx context.Context, client *pixiv.Client, id int64, scope pixiv.Restrict, report *SyncReport) error {
	state, err := s.readBookmarkState(ctx, id, scope)
	if err != nil {
		return err
	}
	if !state.Complete {
		if err = s.bookmarkPages(ctx, client, id, scope, state, false, report); err != nil {
			return err
		}
	}
	return s.bookmarkPages(ctx, client, id, scope, bookmarkState{Complete: true}, true, report)
}

func (s *Store) bookmarkPages(ctx context.Context, client *pixiv.Client, id int64, scope pixiv.Restrict, state bookmarkState, head bool, report *SyncReport) error {
	seen := map[string]bool{}
	restarted := false
	for {
		if seen[state.Cursor.String()] {
			return errors.New("bookmark cursor repeated")
		}
		seen[state.Cursor.String()] = true
		page, err := client.User(id).ArtworkBookmarks(ctx, pixiv.ArtworkBookmarksOptions{Restrict: scope, Cursor: state.Cursor})
		if err != nil {
			if !head && !state.Cursor.IsZero() && !restarted && sdk.ReasonOf(err) == sdk.InvalidArgument {
				state = bookmarkState{}
				seen = map[string]bool{}
				restarted = true
				if err = s.writeBookmarkState(ctx, id, scope, state); err != nil {
					return err
				}
				continue
			}
			return err
		}
		allKnown := true
		for _, artwork := range page.Items {
			var known bool
			if err = s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM bookmark WHERE account_id=? AND restrict=? AND artwork_id=?)`, id, string(scope), fmt.Sprint(artwork.ID)).Scan(&known); err != nil {
				return err
			}
			allKnown = allKnown && known
			value, ok, err := project(artwork, time.Now().UTC())
			if err != nil {
				return err
			}
			if ok {
				if err = s.put(ctx, value); err != nil {
					return err
				}
			}
		}
		state.Cursor = page.Cursor
		if !head {
			state.Complete = page.Cursor.IsZero()
		}
		added, err := s.saveBookmarkBatch(ctx, id, scope, page.Items, state, head)
		if err != nil {
			return err
		}
		report.Bookmarks.Checked += len(page.Items)
		report.Bookmarks.New += added
		if page.Cursor.IsZero() || head && allKnown {
			return nil
		}
	}
}
