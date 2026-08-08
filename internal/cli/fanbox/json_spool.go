package fanbox

import (
	"fmt"
	"io"
	"os"

	clioutput "github.com/FlanChanXwO/pixiv-cli/internal/cli/output"
)

// jsonArraySpool 使网络或编码失败时 stdout 保持为空；只有完整 JSON 文档
// 写入临时文件成功后，才把内容交给调用方 writer。
type jsonArraySpool struct {
	file  *os.File
	first bool
	key   string
}

func newJSONArraySpool(key string) (*jsonArraySpool, error) {
	file, err := os.CreateTemp("", "pixiv-cli-fanbox-json-*.tmp")
	if err != nil {
		return nil, err
	}
	spool := &jsonArraySpool{file: file, first: true, key: key}
	if _, err := fmt.Fprintf(file, "{\n  %q: [", key); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, err
	}
	return spool, nil
}

func appendJSONArray[T any](spool *jsonArraySpool, items []T) error {
	for _, item := range items {
		if spool.first {
			spool.first = false
		} else if _, err := io.WriteString(spool.file, ","); err != nil {
			return err
		}
		encoded, err := clioutput.MarshalJSONValue(item, true)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(spool.file, "\n    %s", encoded); err != nil {
			return err
		}
	}
	return nil
}

func (spool *jsonArraySpool) Commit(out io.Writer) error {
	if _, err := io.WriteString(spool.file, "\n  ]\n}\n"); err != nil {
		return err
	}
	if _, err := spool.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := io.Copy(out, spool.file)
	return err
}

func (spool *jsonArraySpool) Close() {
	if spool == nil || spool.file == nil {
		return
	}
	name := spool.file.Name()
	_ = spool.file.Close()
	_ = os.Remove(name)
}
