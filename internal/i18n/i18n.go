// Package i18n 解析界面语言并读取随程序内嵌的中英文消息。
//
// CLI 按执行选择语言，HTTP 按请求选择语言；本包不保存可变的全局语言。
package i18n

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"golang.org/x/text/language"
)

type contextKey struct{}

// WithLocale 为一次 CLI 执行携带语言，供记录输出等共享调用方使用。
func WithLocale(ctx context.Context, locale Locale) context.Context {
	return context.WithValue(ctx, contextKey{}, locale)
}

// FromContext 读取当前执行的语言；显式零值遵循英文默认值。
func FromContext(ctx context.Context) Locale {
	locale, _ := ctx.Value(contextKey{}).(Locale)
	return locale
}

// Locale 是界面支持的语言；零值使用英文。
type Locale string

const (
	English Locale = "en"
	Chinese Locale = "zh-CN"
)

//go:embed en.json zh-CN.json
var files embed.FS

var catalogs = func() map[Locale]map[string]string {
	all := make(map[Locale]map[string]string)
	for _, locale := range []Locale{English, Chinese} {
		data, err := files.ReadFile(string(locale) + ".json")
		if err != nil {
			panic(err)
		}
		var messages map[string]string
		if err := json.Unmarshal(data, &messages); err != nil {
			panic(err)
		}
		all[locale] = messages
	}
	return all
}()

// Normalize 将中文地区变体归为简体中文，其余语言使用英文。
func Normalize(value string) Locale {
	value = strings.ToLower(strings.ReplaceAll(value, "_", "-"))
	value, _, _ = strings.Cut(value, ".")
	value, _, _ = strings.Cut(value, "@")
	if value == "zh" || strings.HasPrefix(value, "zh-") {
		return Chinese
	}
	return English
}

// System 优先读取 POSIX 环境变量，再读取系统界面语言。
func System() Locale {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := os.Getenv(key); value != "" {
			return Normalize(value)
		}
	}
	return Normalize(nativeLanguage())
}

// AcceptLanguage 按请求中的权重选择支持的语言。
func AcceptLanguage(value string) Locale {
	tags, _, err := language.ParseAcceptLanguage(value)
	if err != nil {
		return English
	}
	for _, tag := range tags {
		base, _ := tag.Base()
		switch base.String() {
		case "zh":
			return Chinese
		case "en":
			return English
		}
	}
	return English
}

// String 返回适用于 HTML lang 和 HTTP Content-Language 的语言标识。
func (l Locale) String() string {
	if l == Chinese {
		return string(Chinese)
	}
	return string(English)
}

// Text 读取内置消息；缺失键是开发错误，须在验收时暴露。
func (l Locale) Text(key string) string {
	value, ok := catalogs[Locale(l.String())][key]
	if !ok {
		panic("missing interface message: " + key)
	}
	return value
}

// CatalogJSON 只导出指定消息域，供自包含网页使用；JSON 编码保留 HTML 转义。
func CatalogJSON(prefix string) []byte {
	all := make(map[Locale]map[string]string)
	for locale, messages := range catalogs {
		selected := make(map[string]string)
		for key, value := range messages {
			if strings.HasPrefix(key, prefix) {
				selected[key] = value
			}
		}
		all[locale] = selected
	}
	data, err := json.Marshal(all)
	if err != nil {
		panic(err)
	}
	return data
}

// messageError 在应用边界翻译提示，原始 Error 保持英文并保留 Go 错误链。
type messageError struct {
	key   string
	args  []any
	cause error
}

func (e *messageError) Error() string { return e.cause.Error() }
func (e *messageError) Unwrap() error { return e.cause }

// Errorf 延迟翻译应用错误，并保留 %w 的包装语义。
func Errorf(key string, args ...any) error {
	return &messageError{key: key, args: args, cause: fmt.Errorf(English.Text(key), args...)}
}

// ErrorText 翻译应用消息，保留上游原因、组合错误与外层上下文。
func (l Locale) ErrorText(err error) string {
	if translated, ok := err.(*messageError); ok {
		args := append([]any(nil), translated.args...)
		for i, arg := range args {
			if cause, ok := arg.(error); ok {
				args[i] = localizedError{error: cause, text: l.ErrorText(cause)}
			}
		}
		return fmt.Errorf(l.Text(translated.key), args...).Error()
	}
	var causes []error
	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		causes = wrapped.Unwrap()
	case interface{ Unwrap() error }:
		if cause := wrapped.Unwrap(); cause != nil {
			causes = []error{cause}
		}
	}
	var replacements []string
	for _, cause := range causes {
		replacements = append(replacements, cause.Error(), l.ErrorText(cause))
	}
	return strings.NewReplacer(replacements...).Replace(err.Error())
}

type localizedError struct {
	error
	text string
}

func (e localizedError) Error() string { return e.text }
