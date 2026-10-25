// Package credential 让账号和浏览器凭据共用字段级脱敏与副本所有权。
package credential

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
)

const redacted = "<redacted>"

// Secret 的凭据只能通过 Value 或 Copy 显式读取；格式化与日志只输出脱敏标记。
// 作为结构体字段时须导出并标记 json:"-"，让 fmt 调用字段的 Formatter。
type Secret struct {
	value []byte
}

// New 复制输入凭据，保留 nil 与空切片的区别。
func New(value []byte) Secret { return Secret{value: bytes.Clone(value)} }

// Copy 返回独立的凭据副本。
func (s Secret) Copy() []byte { return bytes.Clone(s.value) }

// Value 仅供显式导出或认证请求读取凭据。
func (s Secret) Value() string { return string(s.value) }

func (s Secret) Empty() bool { return len(s.value) == 0 }

func (s Secret) String() string { return redacted }

func (s Secret) GoString() string { return redacted }

func (s Secret) Format(state fmt.State, verb rune) {
	if verb == 'q' {
		_, _ = io.WriteString(state, `"`+redacted+`"`)
		return
	}
	_, _ = io.WriteString(state, redacted)
}

func (s Secret) LogValue() slog.Value { return slog.StringValue(redacted) }
