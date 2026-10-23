package detail

import (
	"io"
	"os"

	"github.com/spf13/cobra"
)

// jsonArraySpool 逐条写入 JSON 数组，避免在提交聚合文档前把全部详情结果保存在内存中。
type jsonArraySpool struct {
	file  *os.File
	first bool
}

func newJSONArraySpool() (*jsonArraySpool, error) {
	file, err := os.CreateTemp("", "pixiv-detail-*.json")
	if err != nil {
		return nil, err
	}
	if _, err := file.WriteString("["); err != nil {
		file.Close()
		os.Remove(file.Name())
		return nil, err
	}
	return &jsonArraySpool{file: file, first: true}, nil
}

func (s *jsonArraySpool) Write(value []byte) (int, error) {
	if !s.first {
		if _, err := s.file.WriteString(","); err != nil {
			return 0, err
		}
	}
	s.first = false
	return s.file.Write(value)
}

func (s *jsonArraySpool) commit(out io.Writer) error {
	if _, err := s.file.WriteString("]\n"); err != nil {
		return err
	}
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := io.Copy(out, s.file)
	return err
}

func (s *jsonArraySpool) close() error {
	name := s.file.Name()
	err := s.file.Close()
	removeErr := os.Remove(name)
	if err != nil {
		return err
	}
	return removeErr
}

func markNDJSON(cmd *cobra.Command, enabled bool) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	if enabled {
		cmd.Annotations["pixiv-cli.output-ndjson"] = "true"
	} else {
		cmd.Annotations["pixiv-cli.output-ndjson"] = "false"
	}
}
