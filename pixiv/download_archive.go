package pixiv

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// artworkArchive 只记录完整成功的 artwork ID。它不做预占，因此跨进程并发最多
// 造成重复下载，不会因为崩溃或取消留下会错误跳过的记录。
type artworkArchive struct {
	db *sql.DB
}

func openArtworkArchive(path string) (*artworkArchive, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, invalidResourceError(OperationDownload, "archive path is invalid")
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, invalidResourceError(OperationDownload, "cannot create download archive directory")
	}
	db, err := sql.Open("sqlite", abs)
	if err != nil {
		return nil, invalidResourceError(OperationDownload, "cannot open download archive")
	}
	archive := &artworkArchive{db: db}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS completed_artworks (artwork_id INTEGER PRIMARY KEY)`); err != nil {
		_ = db.Close()
		return nil, invalidResourceError(OperationDownload, "cannot initialize download archive")
	}
	return archive, nil
}

func (a *artworkArchive) Close() error {
	if a == nil || a.db == nil {
		return nil
	}
	if err := a.db.Close(); err != nil {
		return fmt.Errorf("close download archive: %w", err)
	}
	return nil
}

func (a *artworkArchive) Contains(id int64) (bool, error) {
	if a == nil {
		return false, nil
	}
	var found int
	err := a.db.QueryRow(`SELECT 1 FROM completed_artworks WHERE artwork_id = ?`, id).Scan(&found)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, invalidResourceError(OperationDownload, "cannot read download archive")
	}
	return true, nil
}

func (a *artworkArchive) Record(id int64) error {
	if a == nil {
		return nil
	}
	if _, err := a.db.Exec(`INSERT OR IGNORE INTO completed_artworks (artwork_id) VALUES (?)`, id); err != nil {
		return invalidResourceError(OperationDownload, "cannot update download archive")
	}
	return nil
}
