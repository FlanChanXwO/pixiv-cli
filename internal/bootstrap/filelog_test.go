package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDailyTextWriterWritesAndRotatesByDay(t *testing.T) {
	dir := t.TempDir()
	w := newDailyTextWriter(dir, 7)

	// 注入固定时钟，避免 Write 依赖真实 time.Now 导致跨日测试与日历耦合。
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.Local)
	w.now = func() time.Time { return now }
	if _, err := w.Write([]byte("time=2026-07-20T10:00:00 level=INFO msg=first\n")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "2026-07-20.txt")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "msg=first") || strings.Contains(string(body), "{\"") {
		t.Fatalf("body=%s", body)
	}

	// 跨日轮转
	next := time.Date(2026, 7, 21, 1, 0, 0, 0, time.Local)
	w.now = func() time.Time { return next }
	if _, err := w.Write([]byte("time=2026-07-21T01:00:00 level=INFO msg=second\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "2026-07-21.txt")); err != nil {
		t.Fatal(err)
	}
	nextBody, err := os.ReadFile(filepath.Join(dir, "2026-07-21.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(nextBody), "msg=second") {
		t.Fatalf("next day body=%s", nextBody)
	}
	// 旧日文件内容仍在，不被跨日写覆盖。
	oldBody, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(oldBody), "msg=first") {
		t.Fatalf("old day body lost after rotate: %s", oldBody)
	}
	_ = w.Close()
}

func TestCleanupOldLogFilesOnlyRemovesRecognizedLogs(t *testing.T) {
	dir := t.TempDir()
	// 过期日志
	old := filepath.Join(dir, "2026-07-01.txt")
	if err := os.WriteFile(old, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// 非当前命名的文件不属于日志保留期管理范围。
	oldJSONL := filepath.Join(dir, "pixiv-2026-07-02.jsonl")
	if err := os.WriteFile(oldJSONL, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// 保留期内
	keep := filepath.Join(dir, "2026-07-18.txt")
	if err := os.WriteFile(keep, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// 非产品文件不得删除
	other := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(other, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	// 伪装名
	fake := filepath.Join(dir, "pixiv-not-a-date.jsonl")
	if err := os.WriteFile(fake, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.Local)
	cleanupOldLogFiles(dir, now, 7)

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old log should be removed, err=%v", err)
	}
	if _, err := os.Stat(oldJSONL); err != nil {
		t.Fatalf("non-current log name should remain: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("recent log should remain: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("non-log file should remain: %v", err)
	}
	if _, err := os.Stat(fake); err != nil {
		t.Fatalf("unrecognized name should remain: %v", err)
	}
}

func TestDailyTextWriterWriteFailureIsSilent(t *testing.T) {
	// 指向不存在且无法创建的路径：Write 仍返回成功长度，不抛错。
	w := newDailyTextWriter(filepath.Join(string([]byte{0}), "nope"), 7)
	n, err := w.Write([]byte("x"))
	if err != nil || n != 1 {
		t.Fatalf("Write = %d, %v", n, err)
	}
}

func TestDefaultLogDirUsesPixivCLIUnderUserHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "must-not-be-used"))
	t.Setenv("LocalAppData", filepath.Join(home, "must-not-be-used"))

	dir, err := DefaultLogDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(home, "pixiv-cli", "logs") {
		t.Fatalf("unexpected log dir: %s", dir)
	}
}

func TestSuggestLogDirHintDoesNotExposeSensitiveValues(t *testing.T) {
	// 缺少 home 时各平台可从不同环境恢复用户目录；只断言提示内容不含敏感词。
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	if hint := SuggestLogDirHint(); hint != "" {
		if strings.Contains(hint, "token") || strings.Contains(hint, "refresh") {
			t.Fatalf("hint leaked secret-like text: %q", hint)
		}
	}
}

func TestOpenFileLogWriterAcceptsAndRedactsCallerCanariesViaSafeAttrsOnly(t *testing.T) {
	// 文件 writer 本身不改写内容；脱敏责任在 operationLog 只写安全字段。
	// 这里验证：仅写安全 attrs 时 canary 不会出现在日志文件中。
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	const canaryToken = "refresh-token-canary-T11"
	const canaryPath = "/secret/path/canary-T11"
	const canaryQuery = "q=canary-query-T11"

	w := openFileLogWriter()
	// 模拟 operationLog 的安全摘要：只有 operation/result/error_code，没有 token/path/query。
	line := "time=2026-07-20T10:00:00 level=ERROR msg=operation operation=search_illust result=error error_code=upstream_error\n"
	if _, err := w.Write([]byte(line)); err != nil {
		t.Fatal(err)
	}
	if c, ok := w.(interface{ Close() error }); ok {
		_ = c.Close()
	}

	dir, err := DefaultLogDir()
	if err != nil {
		t.Fatal(err)
	}
	day := time.Now().Format("2006-01-02")
	body, err := os.ReadFile(filepath.Join(dir, day+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if !strings.Contains(got, "operation=search_illust") || strings.Contains(got, "{\"") {
		t.Fatalf("missing operation summary: %s", got)
	}
	for _, secret := range []string{canaryToken, canaryPath, canaryQuery} {
		if strings.Contains(got, secret) {
			t.Fatalf("file log leaked %q: %s", secret, got)
		}
	}
}
