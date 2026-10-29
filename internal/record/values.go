package record

import (
	"fmt"
	"strconv"
)

// parsePositiveInt64 把十进制文本解析为正整数；失败时在错误中带上 name 说明字段。
func parsePositiveInt64(value, name string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return id, nil
}
