package vectorindex

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/filelock"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/secretfile"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

type entry struct {
	RecordedAt time.Time     `json:"recorded_at"`
	Record     record.Record `json:"record"`
	Pages      []string      `json:"pages"`
}

// Recorder 把作品快照追加到 Directory 指定的应用根目录，跨进程串行写入。
type Recorder struct{ Directory string }

// Append 保存可见作品的规范快照；漫画只保留第一页。取消或写入失败返回原错误。
func (r Recorder) Append(ctx context.Context, a *pixiv.Artwork) error {
	value, ok, err := project(a, time.Now().UTC())
	if err != nil || !ok {
		return err
	}
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return filelock.WithPrivateLock(ctx, filepath.Join(r.Directory, inboxLockName), func() error {
		directory := Directory(r.Directory)
		if err := secretfile.EnsurePrivateDir(directory); err != nil {
			return err
		}
		return appendPrivate(filepath.Join(directory, inboxName), body)
	})
}

func project(a *pixiv.Artwork, at time.Time) (entry, bool, error) {
	if a.Visible != nil && !*a.Visible {
		return entry{}, false, nil
	}
	value := entry{RecordedAt: at}
	for _, page := range a.Pages {
		u, err := url.Parse(page.Original.URL)
		if err != nil || u.Scheme != "https" || u.Host != "i.pximg.net" {
			return entry{}, false, nil
		}
		value.Pages = append(value.Pages, page.Original.URL)
		if a.ContentType == pixiv.ContentTypeManga {
			break
		}
	}
	if len(value.Pages) == 0 {
		return entry{}, false, nil
	}
	snapshot, err := record.ArtworkOutput(a, false)
	if err != nil {
		return entry{}, false, err
	}
	snapshot.IsBookmarked = nil
	snapshot.IsMuted = nil
	snapshot.Visible = nil

	projected, err := record.RecordFromArtwork(snapshot)
	if err != nil {
		return entry{}, false, err
	}
	output, err := json.Marshal(projected)
	if err != nil {
		return entry{}, false, err
	}
	var fields map[string]any
	if err = json.Unmarshal(output, &fields); err != nil {
		return entry{}, false, err
	}
	delete(fields["user"].(map[string]any), "is_followed")
	output, err = json.Marshal(fields)
	if err != nil {
		return entry{}, false, err
	}
	value.Record, err = record.ParseRecordJSON(output)
	return value, err == nil, err
}

func appendPrivate(path string, body []byte) (err error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, config.PrivateFileMode)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if err = file.Chmod(config.PrivateFileMode); err != nil {
		return err
	}
	if _, err = file.Write(body); err != nil {
		return err
	}
	return file.Sync()
}
