// Package input 解析 CLI 显式声明的 stdin 输入：完整文本值或 NDJSON 记录流。
//
// 包的组成：[Bind] 把 [InputSpec] 挂到命令上并返回本次执行的 [State]。
// 记录消费与失败策略在 records.go。
// 本包不承载 Pixiv 或 FANBOX 的业务语义。
package input

import (
	"fmt"
	"strconv"
)

// PositiveInt64 把十进制文本解析为正整数；失败时在错误中带上 name 说明字段。
func PositiveInt64(value, name string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return id, nil
}
