// Temporary multi-section spool for `pixiv recommended --type all`.
// Command execution lives in all.go.

package recommended

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

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
			return pixivcmd.PrintArtworks(s.file, []pixiv.Artwork{typed})
		case pixiv.Novel:
			return pixivcmd.PrintNovels(s.file, []pixiv.Novel{typed})
		case pixiv.UserPreview:
			return pixivcmd.PrintUserPreviews(s.file, []pixiv.UserPreview{typed})
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
		if err := s.write(item); err != nil {
			return err
		}
	}
	return nil
}

func (s *spool) novels(items []pixiv.Novel) error {
	for _, item := range items {
		if err := s.write(item); err != nil {
			return err
		}
	}
	return nil
}

func (s *spool) users(items []pixiv.UserPreview) error {
	for _, item := range items {
		if err := s.write(item); err != nil {
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
