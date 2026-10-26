// slog handler：按配置输出 text 或 JSON；text 带来源位置和 errors.Join 的顶层原因，
// 两种格式都从 ctx 附上 operation 与 call_id。

package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	FormatText = "text"
	FormatJSON = "json"

	modulePrefix = "github.com/FlanChanXwO/pixiv-cli/"
	timeLayout   = "2006-01-02 15:04:05.000"
)

// New 按配置创建 logger；level 接受 debug、info、warn、error。
func New(w io.Writer, level, format string) (*slog.Logger, error) {
	parsed, err := parseLevel(level)
	if err != nil {
		return nil, err
	}
	options := &slog.HandlerOptions{Level: parsed}
	switch format {
	case FormatText:
		return slog.New(callHandler{inner: newTextHandler(w, parsed)}), nil
	case FormatJSON:
		return slog.New(callHandler{inner: jsonErrorHandler{inner: slog.NewJSONHandler(w, options)}}), nil
	default:
		return nil, fmt.Errorf("unsupported log format %q", format)
	}
}

func parseLevel(level string) (slog.Level, error) {
	switch strings.TrimSpace(level) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", level)
	}
}

// callHandler 把 ctx 中的调用信息附到每条记录，再交给具体格式的 handler。
type callHandler struct {
	inner slog.Handler
}

func (h callHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h callHandler) Handle(ctx context.Context, record slog.Record) error {
	if info, ok := callInfoFrom(ctx); ok {
		record = record.Clone()
		if info.operation != "" {
			record.AddAttrs(slog.String(AttrOperation, info.operation))
		}
		if info.callID != "" {
			record.AddAttrs(slog.String(AttrCallID, info.callID))
		}
	}
	return h.inner.Handle(ctx, record)
}

func (h callHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return callHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h callHandler) WithGroup(name string) slog.Handler {
	return callHandler{inner: h.inner.WithGroup(name)}
}

// jsonErrorHandler 把 error 值展开为带原因树的字符串，避免 JSON 编码成空对象。
type jsonErrorHandler struct {
	inner slog.Handler
}

func (h jsonErrorHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h jsonErrorHandler) Handle(ctx context.Context, record slog.Record) error {
	record = record.Clone()
	attrs := make([]slog.Attr, 0, record.NumAttrs())
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, expandErrorAttr(attr))
		return true
	})
	record = slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.AddAttrs(attrs...)
	return h.inner.Handle(ctx, record)
}

func (h jsonErrorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return jsonErrorHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h jsonErrorHandler) WithGroup(name string) slog.Handler {
	return jsonErrorHandler{inner: h.inner.WithGroup(name)}
}

func expandErrorAttr(attr slog.Attr) slog.Attr {
	if attr.Key != AttrError {
		return attr
	}
	err, ok := attr.Value.Any().(error)
	if !ok {
		return attr
	}
	return slog.String(AttrError, err.Error())
}

type textHandler struct {
	mu     *sync.Mutex
	w      io.Writer
	level  slog.Level
	attrs  []slog.Attr
	groups []string
}

func newTextHandler(w io.Writer, level slog.Level) *textHandler {
	return &textHandler{mu: &sync.Mutex{}, w: w, level: level}
}

func (h *textHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *textHandler) Handle(_ context.Context, record slog.Record) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %-5s --- %s : %s",
		record.Time.Format(timeLayout), record.Level, sourcePath(record.PC), record.Message)

	var errAttr slog.Value
	for _, attr := range h.attrs {
		writeAttr(&b, attr)
		if attr.Key == AttrError {
			errAttr = attr.Value
		}
	}
	record.Attrs(func(attr slog.Attr) bool {
		attr = prefixAttr(h.groups, attr)
		writeAttr(&b, attr)
		if attr.Key == AttrError {
			errAttr = attr.Value
		}
		return true
	})
	b.WriteByte('\n')
	if err, ok := errAttr.Any().(error); ok {
		for _, cause := range errorLines(err) {
			fmt.Fprintf(&b, "    caused by: %s\n", cause)
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

func (h *textHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append([]slog.Attr(nil), h.attrs...)
	for _, attr := range attrs {
		attr.Value = attr.Value.Resolve()
		clone.attrs = append(clone.attrs, prefixAttr(h.groups, attr))
	}
	return &clone
}

func (h *textHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := *h
	clone.groups = append(append([]string(nil), h.groups...), name)
	return &clone
}

func prefixAttr(groups []string, attr slog.Attr) slog.Attr {
	if len(groups) == 0 {
		return attr
	}
	attr.Key = strings.Join(append(append([]string(nil), groups...), attr.Key), ".")
	return attr
}

func writeAttr(b *strings.Builder, attr slog.Attr) {
	if attr.Key == "" && attr.Value.Kind() == slog.KindGroup {
		for _, nested := range attr.Value.Group() {
			writeAttr(b, nested)
		}
		return
	}
	if attr.Key == AttrError {
		if _, ok := attr.Value.Any().(error); ok {
			return
		}
	}
	b.WriteByte(' ')
	b.WriteString(attr.Key)
	b.WriteByte('=')
	b.WriteString(formatValue(attr.Value))
}

func formatValue(value slog.Value) string {
	value = value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		return quoteValue(value.String())
	case slog.KindInt64:
		return strconv.FormatInt(value.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(value.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(value.Float64(), 'g', -1, 64)
	case slog.KindBool:
		return strconv.FormatBool(value.Bool())
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindTime:
		return value.Time().Format(time.RFC3339Nano)
	case slog.KindGroup:
		parts := make([]string, 0, len(value.Group()))
		for _, nested := range value.Group() {
			parts = append(parts, nested.Key+"="+formatValue(nested.Value))
		}
		return strings.Join(parts, ",")
	default:
		if err, ok := value.Any().(error); ok {
			return quoteValue(err.Error())
		}
		return quoteValue(fmt.Sprint(value.Any()))
	}
}

func quoteValue(value string) string {
	if value == "" || strings.ContainsAny(value, "\r\n=\"") {
		return strconv.Quote(value)
	}
	return value
}

func sourcePath(pc uintptr) string {
	if pc == 0 {
		return "unknown"
	}
	frame, _ := runtime.CallersFrames([]uintptr{pc}).Next()
	file := frame.File
	for _, root := range []string{"/internal/", "/sdk/", "/cmd/"} {
		if index := strings.Index(file, root); index >= 0 {
			file = file[index+1:]
			break
		}
	}
	file = strings.TrimPrefix(file, modulePrefix)
	if frame.Line == 0 {
		return file
	}
	return file + ":" + strconv.Itoa(frame.Line)
}

// errorLines 展开 errors.Join 的顶层分支；每条分支只输出自身的安全文本。
// Unwrap 链可能包含上游为 redaction 保留的私有 cause（例如 reverse-search
// provider 的原始响应），不能继续向下展开。
func errorLines(err error) []string {
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return []string{err.Error()}
	}
	lines := make([]string, 0, len(joined.Unwrap()))
	for _, child := range joined.Unwrap() {
		lines = append(lines, child.Error())
	}
	return lines
}
