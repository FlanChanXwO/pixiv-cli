// Package sqliteio 通过系统 sqlite3 命令行工具只读查询本地 SQLite 文件。
//
// 仅由本包树内的 provider 使用；SQL 必须是 provider 控制的常量，参数来自受
// 约束的 CookieQuery。任何失败的错误都是静态分类，绝不包含路径、SQL、命令
// 输出或参数值。
package sqliteio

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"os/exec"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/platform/browsercookies/core"
)

// Query 在 dbPath 上以只读方式执行 sql。参数通过 sqlite3 CLI 的 `.parameter
// set` 绑定（不进行 SQL 字符串拼接）。返回 CSV 行（每行按 SELECT 列序）。
//
// 分类错误：
//   - 数据库被浏览器锁定时返回 core.ErrDatabaseLocked；
//   - sqlite3 命令缺失时返回 core.ErrSQLiteUnavailable；
//   - 其余失败返回 core.ErrQueryFailed。
func Query(ctx context.Context, dbPath, sql string, params map[string]string) ([][]string, error) {
	if strings.TrimSpace(dbPath) == "" || strings.TrimSpace(sql) == "" {
		return nil, core.ErrQueryFailed
	}
	args := []string{"-readonly", "-noheader", "-csv"}
	for name, value := range params {
		if !safeParam(name) || !safeParam(value) {
			return nil, core.ErrQueryFailed
		}
		args = append(args, "-cmd", ".parameter set "+name+" "+value)
	}
	args = append(args, dbPath, sql)
	cmd := exec.CommandContext(ctx, "sqlite3", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, exec.ErrNotFound) {
			return nil, core.ErrSQLiteUnavailable
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && (exitErr.ExitCode() == 5 || strings.Contains(strings.ToLower(stderr.String()), "locked")) {
			return nil, core.ErrDatabaseLocked
		}
		return nil, core.ErrQueryFailed
	}
	reader := csv.NewReader(bytes.NewReader(out))
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, core.ErrQueryFailed
	}
	return rows, nil
}

// safeParam 限制 `.parameter set` 的名称与值字符集，防止参数注入。
func safeParam(s string) bool {
	if s == "" || len(s) > 256 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == '_':
		case r == '@' || r == ':' || r == '$':
		default:
			return false
		}
	}
	return true
}
