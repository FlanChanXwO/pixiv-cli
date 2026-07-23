package bootstrap

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/common/constants"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
)

// defaultLogRetainDays 是按日日志的默认保留天数。
// 仅清理识别出的历史日志文件名，避免误删用户其他文件。
const defaultLogRetainDays = 7

// logFileNamePattern 只识别当前纯文本按日日志，避免保留期清理误删用户文件。
var logFileNamePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\.txt$`)

// DefaultLogDir 返回用户 home 下 pixiv-cli/logs。认证、配置、回调桥接与日志
// 共用同一应用数据根目录；Windows 上 home 对应当前用户 profile。
func DefaultLogDir() (string, error) {
	dir, err := files.UserDataSubdir(constants.AppDataDirName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "logs"), nil
}

// dailyTextWriter 将日志按本地日切到独立纯文本文件。
// 写失败、轮转失败、清理失败一律静默，不影响业务。
type dailyTextWriter struct {
	dir        string
	retainDays int
	// now 仅供测试注入固定时钟；生产路径保持 nil，等价于 time.Now。
	now  func() time.Time
	mu   sync.Mutex
	day  string
	file *os.File
}

func newDailyTextWriter(dir string, retainDays int) *dailyTextWriter {
	if retainDays <= 0 {
		retainDays = defaultLogRetainDays
	}
	return &dailyTextWriter{dir: dir, retainDays: retainDays}
}

func (w *dailyTextWriter) currentTime() time.Time {
	if w.now != nil {
		return w.now()
	}
	return time.Now()
}

func (w *dailyTextWriter) Write(p []byte) (int, error) {
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

func (w *dailyTextWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *dailyTextWriter) ensureFileLocked(now time.Time) error {
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
	path := filepath.Join(w.dir, day+".txt")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w.file = file
	w.day = day
	return nil
}

// cleanupOldLogFiles 只删除识别出的当前日志文件。
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
		day := logFileDay(name)
		if day < cutoff {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

func logFileDay(name string) string {
	return strings.TrimSuffix(name, ".txt")
}

// discardWriteCloser 为日志不可用路径提供无副作用的关闭操作，便于调用方统一管理生命周期。
type discardWriteCloser struct {
	io.Writer
}

func (discardWriteCloser) Close() error { return nil }

// openFileLogWriter 尝试打开默认文件日志 writer；任何失败返回 discard。
// 调用方必须在进程或命令结束时 Close，避免 Windows 持有文本日志文件句柄。
func openFileLogWriter() io.WriteCloser {
	dir, err := DefaultLogDir()
	if err != nil {
		return discardWriteCloser{Writer: io.Discard}
	}
	return newDailyTextWriter(dir, defaultLogRetainDays)
}

// SuggestLogDirHint 仅用于特殊非认证故障的用户提示；失败时返回空串。
func SuggestLogDirHint() string {
	dir, err := DefaultLogDir()
	if err != nil {
		return ""
	}
	return fmt.Sprintf("See log directory: %s", dir)
}
