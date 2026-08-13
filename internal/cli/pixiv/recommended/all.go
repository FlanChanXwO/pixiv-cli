package recommended

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/internal/listing"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// runAll 按既有类别顺序把四个推荐分区写入一份私有临时文档，只有全部读取成功后才
// 把完整文档交给 stdout。因此中途失败不会让用户看到半份结果。
func (a command) runAll(ctx context.Context, client *pixiv.Client, plan listing.Plan, jsonOut bool) (bool, error) {
	spool, err := newSpool(jsonOut)
	if err != nil {
		return false, err
	}
	defer spool.Close()

	fetchArtworks := func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.RecommendedArtworks(ctx, pixiv.RecommendedArtworksRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	for _, section := range []struct {
		key     string
		heading string
	}{{key: "illusts", heading: "recommended illustrations"}, {key: "manga", heading: "recommended manga"}} {
		if err := spool.section(section.key); err != nil {
			return false, err
		}
		if err := spool.heading(jsonOut, section.heading); err != nil {
			return false, err
		}
		if err := listing.PageItems(ctx, plan, fetchArtworks, spool.artworks); err != nil {
			return false, err
		}
	}
	if err := spool.section("novels"); err != nil {
		return false, err
	}
	if err := spool.heading(jsonOut, "recommended novels"); err != nil {
		return false, err
	}
	if err := listing.PageItems(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		result, err := client.RecommendedNovels(ctx, pixiv.RecommendedNovelsRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}, spool.novels); err != nil {
		return false, err
	}
	if err := spool.section("user_previews"); err != nil {
		return false, err
	}
	if err := spool.heading(jsonOut, "recommended users"); err != nil {
		return false, err
	}
	if err := listing.PageItems(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		result, err := client.RecommendedUsers(ctx, pixiv.RecommendedUsersRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}, spool.users); err != nil {
		return false, err
	}
	// 最终 io.Copy 可能已交付部分 stdout；在尝试前关闭账号池重放窗口。
	return true, spool.Commit(a.data.Output)
}

// spool 是 `recommended all` 独有的多分区临时文档。单一实体列表使用共享 listing
// 的单键数组 spool，因此这里只保留跨分区的顺序与分隔语义。
type spool struct {
	file                             *os.File
	jsonOut, firstSection, firstItem bool
}

// WriteSpoolHeader 是推荐结果临时文档的首段写入 seam。它保持为 package 变量，
// 便于 CLI 集成测试验证临时文件失败时不会泄露半份 stdout。
var WriteSpoolHeader = io.WriteString

func newSpool(jsonOut bool) (*spool, error) {
	file, err := os.CreateTemp("", "pixiv-cli-recommended-*.tmp")
	if err != nil {
		return nil, err
	}
	s := &spool{file: file, jsonOut: jsonOut, firstSection: true, firstItem: true}
	if jsonOut {
		if _, err := WriteSpoolHeader(file, "{"); err != nil {
			name := file.Name()
			_ = file.Close()
			_ = os.Remove(name)
			return nil, err
		}
	}
	return s, nil
}

func (s *spool) section(key string) error {
	s.firstItem = true
	if !s.jsonOut {
		return nil
	}
	if !s.firstSection {
		if _, err := io.WriteString(s.file, "\n  ],"); err != nil {
			return err
		}
	}
	s.firstSection = false
	_, err := fmt.Fprintf(s.file, "\n  %q: [", key)
	return err
}

func (s *spool) heading(jsonOut bool, text string) error {
	if jsonOut {
		return nil
	}
	_, err := fmt.Fprintln(s.file, text)
	return err
}

func (s *spool) write(value any) error {
	if !s.jsonOut {
		switch typed := value.(type) {
		case pixiv.Artwork:
			return printArtworks(s.file, []pixiv.Artwork{typed})
		case pixiv.Novel:
			return printNovels(s.file, []pixiv.Novel{typed})
		case pixiv.UserPreview:
			return printUserPreviews(s.file, []pixiv.UserPreview{typed})
		}
		return nil
	}
	if !s.firstItem {
		if _, err := io.WriteString(s.file, ","); err != nil {
			return err
		}
	}
	s.firstItem = false
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(s.file, "\n    %s", body)
	return err
}

func (s *spool) artworks(items []pixiv.Artwork) error {
	for _, item := range items {
		value := any(item)
		if s.jsonOut {
			value = pixiv.ToArtworkDTO(item)
		}
		if err := s.write(value); err != nil {
			return err
		}
	}
	return nil
}

func (s *spool) novels(items []pixiv.Novel) error {
	for _, item := range items {
		value := any(item)
		if s.jsonOut {
			value = pixiv.ToNovelDTO(item)
		}
		if err := s.write(value); err != nil {
			return err
		}
	}
	return nil
}

func (s *spool) users(items []pixiv.UserPreview) error {
	for _, item := range items {
		value := any(item)
		if s.jsonOut {
			value = pixiv.ToUserPreviewDTO(item)
		}
		if err := s.write(value); err != nil {
			return err
		}
	}
	return nil
}

func (s *spool) Commit(out io.Writer) error {
	if s.jsonOut {
		if _, err := io.WriteString(s.file, "\n  ]\n}\n"); err != nil {
			return err
		}
	}
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := io.Copy(out, s.file)
	return err
}

func (s *spool) Close() {
	if s == nil || s.file == nil {
		return
	}
	name := s.file.Name()
	_ = s.file.Close()
	_ = os.Remove(name)
}
