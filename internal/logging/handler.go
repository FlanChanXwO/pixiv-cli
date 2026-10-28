package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/term"
)

const (
	// FormatText 输出带来源位置与人读错误链的文本日志。
	FormatText = "text"
	// FormatJSON 输出单行 JSON 日志对象。
	FormatJSON = "json"

	modulePrefix = "github.com/FlanChanXwO/pixiv-cli/"
	timeLayout   = "15:04:05.000"

	colorReset  = "\x1b[0m"
	colorGreen  = "\x1b[32m"
	colorBlue   = "\x1b[34m"
	colorCyan   = "\x1b[36m"
	colorYellow = "\x1b[33m"
	colorRed    = "\x1b[31m"
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
		options.AddSource = true
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
	color  bool
	attrs  []slog.Attr
	groups []string
}

func newTextHandler(w io.Writer, level slog.Level) *textHandler {
	return &textHandler{mu: &sync.Mutex{}, w: w, level: level, color: colorEnabled(w)}
}

// colorEnabled 只在 stderr 是终端且未设置 NO_COLOR 时着色。
func colorEnabled(w io.Writer) bool {
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled {
		return false
	}
	file, ok := w.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func (h *textHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *textHandler) Handle(_ context.Context, record slog.Record) error {
	var b strings.Builder
	b.WriteByte('[')
	if h.color {
		b.WriteString(colorGreen)
	}
	b.WriteString(record.Time.Local().Format(timeLayout))
	if h.color {
		b.WriteString(colorReset)
	}
	b.WriteByte(']')

	if operation := h.operation(record); operation != "" {
		fmt.Fprintf(&b, " [%s]", quoteText(operation))
	}
	b.WriteString(" [")
	if h.color {
		b.WriteString(levelColor(record.Level))
	}
	b.WriteString(levelLabel(record.Level))
	if h.color {
		b.WriteString(colorReset)
	}
	b.WriteString("]")
	fmt.Fprintf(&b, " [%s]: ", quoteText(sourcePath(record.PC)))
	b.WriteString(quoteText(record.Message))

	attrs := append([]slog.Attr(nil), h.attrs...)
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == AttrOperation {
			attrs = append(attrs, attr)
			return true
		}
		attrs = append(attrs, prefixAttr(h.groups, attr))
		return true
	})
	var errAttr slog.Value
	var normal, multiline []string
	for _, attr := range attrs {
		if attr.Key == AttrError {
			errAttr = attr.Value
		}
		text := attrText(attr)
		if text == "" {
			continue
		}
		if strings.Contains(text, "\n") {
			multiline = append(multiline, text)
		} else {
			normal = append(normal, text)
		}
	}
	for _, text := range normal {
		b.WriteString(text)
	}
	// 多行值留在行尾，后续行才不会被同一行的其他属性打断。
	for _, text := range multiline {
		b.WriteString(text)
	}
	b.WriteByte('\n')
	if err, ok := errAttr.Any().(error); ok {
		for _, cause := range errorLines(err) {
			fmt.Fprintf(&b, "    caused by: %s\n", quoteText(cause))
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

// operation 从调用信息或显式属性中取 operation 标签；文本格式由标签承载它。
func (h *textHandler) operation(record slog.Record) string {
	for _, attr := range h.attrs {
		if attr.Key == AttrOperation {
			return attr.Value.Resolve().String()
		}
	}
	var operation string
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == AttrOperation {
			operation = attr.Value.Resolve().String()
			return false
		}
		return true
	})
	return operation
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

func attrText(attr slog.Attr) string {
	var b strings.Builder
	writeAttr(&b, attr)
	return b.String()
}

func writeAttr(b *strings.Builder, attr slog.Attr) {
	if attr.Key == "" && attr.Value.Kind() == slog.KindGroup {
		for _, nested := range attr.Value.Group() {
			writeAttr(b, nested)
		}
		return
	}
	if attr.Key == AttrOperation {
		return
	}
	if attr.Key == AttrError {
		if _, ok := attr.Value.Any().(error); ok {
			return
		}
	}
	b.WriteByte(' ')
	b.WriteString(formatKey(attr.Key))
	b.WriteByte('=')
	b.WriteString(formatValue(attr.Value))
}

func formatKey(key string) string {
	if key == "" || hasNonPrintable(key) || strings.ContainsAny(key, " \t\"=") {
		return strconv.Quote(key)
	}
	return key
}

func formatValue(value slog.Value) string {
	value = value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		return formatString(value.String())
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
			parts = append(parts, formatKey(nested.Key)+"="+formatValue(nested.Value))
		}
		return strings.Join(parts, ",")
	default:
		if err, ok := value.Any().(error); ok {
			return formatString(err.Error())
		}
		return formatString(fmt.Sprint(value.Any()))
	}
}

// formatString 给空值、含空白或引号的值加引号；多行值保留换行，后续行缩进 4 个空格。
func formatString(value string) string {
	if strings.Contains(value, "\n") {
		lines := strings.Split(value, "\n")
		quoted := make([]string, 0, len(lines))
		for _, line := range lines {
			escaped := strconv.Quote(line)
			quoted = append(quoted, escaped[1:len(escaped)-1])
		}
		return `"` + strings.Join(quoted, "\n    ") + `"`
	}
	if value == "" || strings.ContainsAny(value, `"`) || hasNonPrintable(value) || strings.ContainsFunc(value, unicode.IsSpace) {
		return strconv.Quote(value)
	}
	return value
}

// quoteText 只转义不可打印内容；普通文本（包括空格）保持原样。
func quoteText(value string) string {
	if hasNonPrintable(value) {
		return strconv.Quote(value)
	}
	return value
}

func hasNonPrintable(value string) bool {
	return strings.ContainsFunc(value, func(r rune) bool {
		return !unicode.IsPrint(r)
	})
}

func levelLabel(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "ERRO"
	case level >= slog.LevelWarn:
		return "WARN"
	case level >= slog.LevelInfo:
		return "INFO"
	default:
		return "DBUG"
	}
}

func levelColor(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return colorRed
	case level >= slog.LevelWarn:
		return colorYellow
	case level >= slog.LevelInfo:
		return colorCyan
	default:
		return colorBlue
	}
}

func sourcePath(pc uintptr) string {
	if pc == 0 {
		return "unknown"
	}
	frame, _ := runtime.CallersFrames([]uintptr{pc}).Next()
	file := frame.File
	if _, after, ok := strings.Cut(file, modulePrefix); ok {
		file = after
	}
	if index := strings.LastIndexByte(file, '/'); index >= 0 {
		if parent := strings.LastIndexByte(file[:index], '/'); parent >= 0 {
			file = file[parent+1:]
		}
	}
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
