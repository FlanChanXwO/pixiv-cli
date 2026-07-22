package bootstrap

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// defaultLogRetainDays 是按日日志的默认保留天数。
// 仅清理识别出的历史日志文件名，避免误删用户其他文件。
const defaultLogRetainDays = 7

// logFileNamePattern 仅匹配本产品写入的按日 JSONL 日志。
var logFileNamePattern = regexp.MustCompile(`^pixiv-\d{4}-\d{2}-\d{2}\.jsonl$`)

// DefaultLogDir 返回用户 state 目录下的 pixiv/logs。
// 当前工具链未提供 os.UserStateDir，这里按同一语义实现：
// Unix 使用 $XDG_STATE_HOME 或 $HOME/.local/state；
// Darwin 使用 $HOME/Library/Application Support；
// Windows 使用 %LocalAppData%。
// 目录创建失败时返回 error，调用方应静默继续。
func DefaultLogDir() (string, error) {
	state, err := userStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(state, "pixiv", "logs"), nil
}

func userStateDir() (string, error) {
	switch runtime.GOOS {
	case "windows":
		dir := os.Getenv("LocalAppData")
		if dir == "" {
			return "", fmt.Errorf("%%LocalAppData%% is not defined")
		}
		return dir, nil
	case "darwin", "ios":
		home := os.Getenv("HOME")
		if home == "" {
			return "", fmt.Errorf("$HOME is not defined")
		}
		return filepath.Join(home, "Library", "Application Support"), nil
	default:
		dir := os.Getenv("XDG_STATE_HOME")
		if dir != "" {
			if !filepath.IsAbs(dir) {
				return "", fmt.Errorf("path in $XDG_STATE_HOME is relative")
			}
			return dir, nil
		}
		home := os.Getenv("HOME")
		if home == "" {
			return "", fmt.Errorf("neither $XDG_STATE_HOME nor $HOME are defined")
		}
		return filepath.Join(home, ".local", "state"), nil
	}
}

// dailyJSONLWriter 将日志按本地日切到独立 JSONL 文件。
// 写失败、轮转失败、清理失败一律静默，不影响业务。
type dailyJSONLWriter struct {
	dir        string
	retainDays int
	// now 仅供测试注入固定时钟；生产路径保持 nil，等价于 time.Now。
	now  func() time.Time
	mu   sync.Mutex
	day  string
	file *os.File
}

func newDailyJSONLWriter(dir string, retainDays int) *dailyJSONLWriter {
	if retainDays <= 0 {
		retainDays = defaultLogRetainDays
	}
	return &dailyJSONLWriter{dir: dir, retainDays: retainDays}
}

func (w *dailyJSONLWriter) currentTime() time.Time {
	if w.now != nil {
		return w.now()
	}
	return time.Now()
}

func (w *dailyJSONLWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.ensureFileLocked(w.currentTime()); err != nil {
		// 日志不得拖垮业务：写失败当作已消费。
		return len(p), nil
	}
	n, err := w.file.Write(p)
	if err != nil {
		return len(p), nil
	}
	return n, nil
}

func (w *dailyJSONLWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *dailyJSONLWriter) ensureFileLocked(now time.Time) error {
	day := now.Format("2006-01-02")
	if w.file != nil && w.day == day {
		return nil
	}
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return err
	}
	// 轮转后尽力清理过期日志；失败静默。
	cleanupOldLogFiles(w.dir, now, w.retainDays)
	path := filepath.Join(w.dir, "pixiv-"+day+".jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w.file = file
	w.day = day
	return nil
}

// cleanupOldLogFiles 只删除识别出的 pixiv-YYYY-MM-DD.jsonl 历史文件。
func cleanupOldLogFiles(dir string, now time.Time, retainDays int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := now.AddDate(0, 0, -retainDays).Format("2006-01-02")
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !logFileNamePattern.MatchString(name) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		// pixiv-YYYY-MM-DD.jsonl
		day := strings.TrimSuffix(strings.TrimPrefix(name, "pixiv-"), ".jsonl")
		if day < cutoff {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

// discardWriteCloser 为日志不可用路径提供无副作用的关闭操作，便于调用方统一管理生命周期。
type discardWriteCloser struct {
	io.Writer
}

func (discardWriteCloser) Close() error { return nil }

// openFileLogWriter 尝试打开默认文件日志 writer；任何失败返回 discard。
// 调用方必须在进程或命令结束时 Close，避免 Windows 持有 JSONL 文件句柄。
func openFileLogWriter() io.WriteCloser {
	dir, err := DefaultLogDir()
	if err != nil {
		return discardWriteCloser{Writer: io.Discard}
	}
	return newDailyJSONLWriter(dir, defaultLogRetainDays)
}

// SuggestLogDirHint 仅用于特殊非认证故障的用户提示；失败时返回空串。
func SuggestLogDirHint() string {
	dir, err := DefaultLogDir()
	if err != nil {
		return ""
	}
	return fmt.Sprintf("See log directory: %s", dir)
}
