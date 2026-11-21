package vectorindex

import (
	"bufio"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/filelock"
)

// Counts 保存当前模型的页面处理数量。
type Counts struct {
	Artworks    int `json:"artworks"`
	Pages       int `json:"pages"`
	Pending     int `json:"pending"`
	Unavailable int `json:"unavailable"`
	Failed      int `json:"failed"`
}

// Status 汇总本地状态；Disk 的大小单位为字节，Models 按模型标识统计成功页面。
type Status struct {
	Enabled   bool              `json:"enabled"`
	Model     string            `json:"model"`
	Index     Counts            `json:"index"`
	Recorded  int               `json:"recorded"`
	Models    map[string]int    `json:"models"`
	Bookmarks map[string]string `json:"bookmarks"`
	Ranges    map[string]string `json:"ranges,omitempty"`
	Disk      map[string]int64  `json:"disk"`
}

// ReadStatus 读取已有索引和待合并记录；缺少索引时返回实际磁盘状态。
func ReadStatus(ctx context.Context, root, model string) (result Status, err error) {
	result = Status{Enabled: model != "", Model: model, Models: map[string]int{}, Bookmarks: map[string]string{}, Disk: map[string]int64{}}
	directory := Directory(root)
	if _, err = os.Stat(filepath.Join(directory, indexName)); err == nil {
		var store *Store
		store, err = Open(ctx, root)
		if err != nil {
			return result, err
		}
		defer func() { err = errors.Join(err, store.Close()) }()
		if result.Index, err = store.Counts(ctx, model); err != nil {
			return result, err
		}
		rows, err := store.db.QueryContext(ctx, `SELECT model,count(*) FROM embedding WHERE vector IS NOT NULL GROUP BY model`)
		if err != nil {
			return result, err
		}
		for rows.Next() {
			var name string
			var n int
			if err := rows.Scan(&name, &n); err != nil {
				return result, errors.Join(err, rows.Close())
			}
			result.Models[name] = n
		}
		if err = errors.Join(rows.Err(), rows.Close()); err != nil {
			return result, err
		}
		rows, err = store.db.QueryContext(ctx, `SELECT key,value FROM state`)
		if err != nil {
			return result, err
		}
		for rows.Next() {
			var key, value string
			if err := rows.Scan(&key, &value); err != nil {
				return result, errors.Join(err, rows.Close())
			}
			if strings.HasPrefix(key, rangePrefix) {
				if result.Ranges == nil {
					result.Ranges = map[string]string{}
				}
				result.Ranges[key] = value
			} else {
				result.Bookmarks[key] = value
			}
		}
		if err = errors.Join(rows.Err(), rows.Close()); err != nil {
			return result, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	for _, name := range []string{inboxName, mergingName} {
		n, err := countLines(filepath.Join(directory, name))
		if err != nil {
			return result, err
		}
		result.Recorded += n
	}
	for _, name := range []string{indexName, "models", "runtime"} {
		n, err := DiskUsage(filepath.Join(directory, name))
		if err != nil {
			return result, err
		}
		result.Disk[name] = n
	}
	return result, nil
}

// Counts 统计指定模型的成功、待算和永久失败页面。
func (s *Store) Counts(ctx context.Context, model string) (result Counts, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM artwork),count(e.vector),coalesce(sum(p.unavailable='' AND e.model IS NULL),0),coalesce(sum(p.unavailable<>''),0),coalesce(sum(e.failure<>''),0) FROM page p LEFT JOIN embedding e ON e.artwork_id=p.artwork_id AND e.page_index=p.page_index AND e.model=?`, model).Scan(&result.Artworks, &result.Pages, &result.Pending, &result.Unavailable, &result.Failed)
	return result, err
}

func countLines(path string) (n int, err error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	r := bufio.NewReader(f)
	for {
		line, e := r.ReadBytes('\n')
		if len(line) > 0 {
			n++
		}
		if errors.Is(e, io.EOF) {
			return n, nil
		}
		if e != nil {
			return n, e
		}
	}
}

// DiskUsage 统计指定文件或目录的实际逻辑字节数；路径缺失视为零。
func DiskUsage(path string) (n int64, err error) {
	err = filepath.WalkDir(path, func(_ string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !e.IsDir() {
			info, err := e.Info()
			if err != nil {
				return err
			}
			n += info.Size()
		}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	return n, err
}

// Clear 删除索引或完整向量目录。调用方须持有同步锁并完成确认。
func Clear(ctx context.Context, root string, all bool) error {
	return filelock.WithPrivateLock(ctx, filepath.Join(root, inboxLockName), func() error {
		directory := Directory(root)
		if all {
			return os.RemoveAll(directory)
		}
		for _, name := range []string{indexName, indexName + "-journal", inboxName, mergingName, rejectedName} {
			if err := os.Remove(filepath.Join(directory, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		return nil
	})
}

// ClearUnused 删除其他模型的向量；调用方须持有同步锁。
func (s *Store) ClearUnused(ctx context.Context, model string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM embedding WHERE model<>?`, model)
	return err
}
