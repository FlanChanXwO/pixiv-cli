// Package display 渲染 CLI 的终端文本：实体列表行、详情卡片、身份格式与上游文本转义。
//
// 本包只供 internal/cli 使用；JSON 与 NDJSON 由 internal/record 输出，不经过这里。
// 只服务一个命令的渲染留在该命令目录。
//
// 包的组成：[SafeLine] 把上游文本转成可安全打印的单行，
// [Identity] 输出 `用户名 (UID)`，作品、小说与用户的列表行和详情卡片在同一包内。
package display

import "strconv"

// Identity 把用户名与 UID 写成 `用户名 (UID)`；用户名为空时只写 UID。
func Identity(username string, userID int64) string {
	id := strconv.FormatInt(userID, 10)
	if username == "" {
		return id
	}
	return SafeLine(username) + " (" + id + ")"
}
