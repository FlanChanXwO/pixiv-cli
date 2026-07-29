package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TextHandler 把安全的结构化事件渲染为适合直接阅读的单行日志。它只服务本地
// 文件日志；调用方自行注入的 JSON handler 仍可取得完整、稳定的事件字段。
//
// 行首采用 Spring Boot/SLF4J 常见的列式布局：时间、级别、PID、业务组件与调用点。
// source 是 LogOperation/LogRateLimitRetry 的受控调用点，operation 留在消息中。
// 不使用 slog 的 AddSource：它会指向该公共封装；本包自行跳过封装栈帧，记录真正
// 发起操作的仓库相对文件与行号。
type TextHandler struct {
	writer io.Writer
	level  slog.Leveler
	attrs  []slog.Attr
	groups []string
	pid    int
	mu     *sync.Mutex
}

// DefaultTextTemplate 是本地文本日志的唯一行模板。变量名与模板分离，便于未来
// 调整列顺序、宽度或新增安全字段，而不让格式逻辑散落在 handler 中。
const DefaultTextTemplate = "{time}  {level} {pid} --- [{component}] {source} : {message}{details}\n"

var textTemplatePlaceholders = []string{
	"{time}",
	"{level}",
	"{pid}",
	"{component}",
	"{source}",
	"{message}",
	"{details}",
}

// NewTextHandler 返回项目本地日志使用的紧凑文本 handler。options 目前只采用
// Level；日志字段由本包的安全事件协议控制，不允许 ReplaceAttr 改写或放行字段。
func NewTextHandler(writer io.Writer, options *slog.HandlerOptions) *TextHandler {
	var level slog.Leveler = slog.LevelInfo
	if options != nil && options.Level != nil {
		level = options.Level
	}
	return &TextHandler{writer: writer, level: level, pid: os.Getpid(), mu: &sync.Mutex{}}
}

func (h *TextHandler) Enabled(_ context.Context, level slog.Level) bool {
	if h == nil || h.level == nil {
		return false
	}
	return level >= h.level.Level()
}

func (h *TextHandler) Handle(_ context.Context, record slog.Record) error {
	if h == nil || h.writer == nil {
		return nil
	}
	fields := make([]textField, 0, len(h.attrs)+record.NumAttrs())
	for _, attr := range h.attrs {
		fields = appendTextAttr(fields, h.groups, attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		fields = appendTextAttr(fields, h.groups, attr)
		return true
	})

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.writer, h.format(record, fields))
	return err
}

func (h *TextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return &clone
}

func (h *TextHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := *h
	clone.groups = append(append([]string(nil), h.groups...), name)
	return &clone
}

type textField struct {
	key   string
	value slog.Value
}

func appendTextAttr(fields []textField, groups []string, attr slog.Attr) []textField {
	attr.Value = attr.Value.Resolve()
	if attr.Value.Kind() != slog.KindGroup {
		if attr.Key == "" {
			return fields
		}
		return append(fields, textField{key: strings.Join(append(append([]string(nil), groups...), attr.Key), "."), value: attr.Value})
	}

	nestedGroups := groups
	if attr.Key != "" {
		nestedGroups = append(append([]string(nil), groups...), attr.Key)
	}
	for _, nested := range attr.Value.Group() {
		fields = appendTextAttr(fields, nestedGroups, nested)
	}
	return fields
}

func (h *TextHandler) format(record slog.Record, fields []textField) string {
	values := make(map[string]slog.Value, len(fields))
	for _, field := range fields {
		// 与 slog 的同名属性输出顺序一致，后出现的值代表当前 record 的覆写。
		values[field.key] = field.value
	}

	component := textValue(values[LogFieldComponent])
	if component == "" {
		component = "main"
	}

	var message, details string
	switch record.Message {
	case OperationLogMessage:
		message, details = h.formatOperation(values, fields)
	case RateLimitRetryLogMessage:
		message, details = h.formatRateLimitRetry(values, fields)
	default:
		var extra strings.Builder
		h.appendExtraFields(&extra, fields, genericFieldKeys)
		message, details = record.Message, extra.String()
	}

	return renderTextTemplate(DefaultTextTemplate, map[string]string{
		"{time}":      record.Time.Format("2006-01-02 15:04:05.000"),
		"{level}":     fmt.Sprintf("%-5s", textLevel(record.Level)),
		"{pid}":       strconv.Itoa(h.pid),
		"{component}": component,
		"{source}":    paddedLogSource(logSource(record.Message, values)),
		"{message}":   message,
		"{details}":   details,
	})
}

func renderTextTemplate(template string, variables map[string]string) string {
	replacements := make([]string, 0, len(textTemplatePlaceholders)*2)
	for _, placeholder := range textTemplatePlaceholders {
		replacements = append(replacements, placeholder, variables[placeholder])
	}
	return strings.NewReplacer(replacements...).Replace(template)
}

// logSource 对应 Spring/SLF4J 行中的 logger 名。operation event 优先显示受控的
// 调用点；手工构造的兼容事件仍可回退到 operation，普通日志使用 pixiv。
func logSource(message string, values map[string]slog.Value) string {
	if message == OperationLogMessage || message == RateLimitRetryLogMessage {
		if source := textValue(values[LogFieldSource]); source != "" {
			return source
		}
		if operation := textValue(values[LogFieldOperation]); operation != "" {
			return operation
		}
	}
	return "pixiv"
}

func paddedLogSource(source string) string {
	const sourceColumnWidth = 36
	var column strings.Builder
	column.WriteString(source)
	if padding := sourceColumnWidth - len(source); padding > 0 {
		column.WriteString(strings.Repeat(" ", padding))
	}
	return column.String()
}

func (h *TextHandler) formatOperation(values map[string]slog.Value, fields []textField) (string, string) {
	operation := textValue(values[LogFieldOperation])
	if operation == "" {
		operation = "operation"
	}
	result := textValue(values[LogFieldResult])
	resultMessage := result
	if result == "" || result == ResultSuccess {
		resultMessage = "completed"
	}
	message := operation + " " + resultMessage

	var details strings.Builder
	if backend := textValue(values[LogFieldBackend]); backend != "" && backend != BackendLocal {
		appendTextField(&details, "backend", backend)
	}
	if code := textValue(values[LogFieldErrorCode]); code != "" {
		appendTextField(&details, "error", code)
	}
	if status := textInt(values[LogFieldStatus]); status != 0 {
		appendTextField(&details, "status", strconv.FormatInt(status, 10))
	}
	if transport := textValue(values[LogFieldTransportKind]); transport != "" {
		appendTextField(&details, "transport", transport)
	}
	if illustID := textInt(values[LogFieldIllustID]); illustID != 0 {
		appendTextField(&details, LogFieldIllustID, strconv.FormatInt(illustID, 10))
	}
	if userID := textInt(values[LogFieldUserID]); userID != 0 {
		appendTextField(&details, LogFieldUserID, strconv.FormatInt(userID, 10))
	}
	if duration := textDuration(values[LogFieldDuration]); duration != 0 {
		appendTextField(&details, LogFieldDuration, duration.String())
	}
	h.appendExtraFields(&details, fields, operationFieldKeys)
	return message, details.String()
}

func (h *TextHandler) formatRateLimitRetry(values map[string]slog.Value, fields []textField) (string, string) {
	var details strings.Builder
	if retryAfter := textDuration(values[LogFieldRetryAfter]); retryAfter != 0 {
		appendTextField(&details, "after", retryAfter.String())
	}
	if attempt := textInt(values[LogFieldAttempt]); attempt != 0 {
		appendTextField(&details, LogFieldAttempt, strconv.FormatInt(attempt, 10))
	}
	if status := textInt(values[LogFieldStatus]); status != 0 {
		appendTextField(&details, "status", strconv.FormatInt(status, 10))
	}
	h.appendExtraFields(&details, fields, rateLimitFieldKeys)
	return "rate-limit retry", details.String()
}

var genericFieldKeys = map[string]struct{}{
	LogFieldComponent: {},
	LogFieldSource:    {},
}

var operationFieldKeys = map[string]struct{}{
	LogFieldComponent:     {},
	LogFieldOperation:     {},
	LogFieldSource:        {},
	LogFieldBackend:       {},
	LogFieldDuration:      {},
	LogFieldResult:        {},
	LogFieldErrorCode:     {},
	LogFieldStatus:        {},
	LogFieldTransportKind: {},
	LogFieldIllustID:      {},
	LogFieldUserID:        {},
}

var rateLimitFieldKeys = map[string]struct{}{
	LogFieldComponent:  {},
	LogFieldOperation:  {},
	LogFieldSource:     {},
	LogFieldResult:     {},
	LogFieldStatus:     {},
	LogFieldRetryAfter: {},
	LogFieldAttempt:    {},
}

func (h *TextHandler) appendExtraFields(line *strings.Builder, fields []textField, known map[string]struct{}) {
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if _, duplicate := seen[field.key]; duplicate {
			continue
		}
		seen[field.key] = struct{}{}
		if known != nil {
			if _, handled := known[field.key]; handled {
				continue
			}
		}
		appendTextField(line, field.key, textValue(field.value))
	}
}

func appendTextField(line *strings.Builder, key, value string) {
	line.WriteByte(' ')
	line.WriteString(key)
	line.WriteByte('=')
	line.WriteString(quoteTextValue(value))
}

func quoteTextValue(value string) string {
	if value == "" || strings.ContainsAny(value, " \t\r\n=\"[]") {
		return strconv.Quote(value)
	}
	return value
}

func textLevel(level slog.Level) string {
	switch {
	case level <= slog.LevelDebug-4:
		return "TRACE"
	case level <= slog.LevelDebug:
		return "DEBUG"
	case level < slog.LevelWarn:
		return "INFO"
	case level < slog.LevelError:
		return "WARN"
	default:
		return "ERROR"
	}
}

func textValue(value slog.Value) string {
	switch value.Kind() {
	case slog.KindString:
		return value.String()
	case slog.KindBool:
		return strconv.FormatBool(value.Bool())
	case slog.KindInt64:
		return strconv.FormatInt(value.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(value.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(value.Float64(), 'g', -1, 64)
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindTime:
		return value.Time().Format(time.RFC3339Nano)
	case slog.KindAny:
		if value.Any() == nil {
			return ""
		}
		return fmt.Sprint(value.Any())
	default:
		return ""
	}
}

func textInt(value slog.Value) int64 {
	switch value.Kind() {
	case slog.KindInt64:
		return value.Int64()
	case slog.KindUint64:
		if value.Uint64() <= uint64(^uint64(0)>>1) {
			return int64(value.Uint64())
		}
	}
	return 0
}

func textDuration(value slog.Value) time.Duration {
	if value.Kind() == slog.KindDuration {
		return value.Duration()
	}
	return 0
}
