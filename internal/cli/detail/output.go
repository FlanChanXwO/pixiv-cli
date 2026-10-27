package detail

import (
	"io"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
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

// Write 把一条已编码的 JSON 值追加进数组，必要时补逗号。
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

// markNDJSON 记录 detail 的输出模式，只在确实要写记录流时安装 broken-pipe 策略。
func (a commandRunner) markNDJSON(cmd *cobra.Command, enabled bool) {
	command.SetOutputNDJSON(cmd, enabled)
	if enabled && a.env.EnablePipelineSignal != nil {
		a.env.EnablePipelineSignal()
	}
}
