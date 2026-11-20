package environment

import (
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
)

// PositiveInt64 把十进制文本解析为正整数；失败时在错误中带上 name 说明字段。
func PositiveInt64(value, name string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, i18n.Errorf("error.must_be_a_positive_integer", name)
	}
	return id, nil
}
