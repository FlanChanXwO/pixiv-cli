package secret

import (
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
)

// ReadJSON 读取私有 JSON 状态；不存在是正常的首次运行状态，空文件则是明确损坏。
func ReadJSON(path string, value any) (bool, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(string(body)) == "" {
		return false, errors.New("local state file is empty")
	}
	if err := json.Unmarshal(body, value); err != nil {
		return false, err
	}
	return true, nil
}

// WriteJSON 用已有的私有文件原子写入协议写状态，不让调用方自行处理权限或替换。
func WriteJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return WritePrivateFile(path, body, paths.PrivateFileMode)
}
