// origin.go 保存 SDK 错误的创建位置，供执行边界读取而无需扩展 SDK 公开方法集。
package logging

import (
	"errors"
	"log/slog"
	"runtime"
	"strings"
)

// ErrorOrigin 以未导出方法跨包传递位置；SDK 通过私有别名嵌入，JSON 无可序列化字段。
type ErrorOrigin struct{ pc uintptr }

func (o ErrorOrigin) errorOrigin() string {
	if o.pc == 0 {
		return ""
	}
	return sourcePath(o.pc)
}

// CaptureErrorOrigin 跳过 SDK 构造函数及产品 errors.go 中的分类包装。
func CaptureErrorOrigin() ErrorOrigin {
	for skip := 2; ; skip++ {
		var pcs [1]uintptr
		if runtime.Callers(skip, pcs[:]) == 0 {
			return ErrorOrigin{}
		}
		frame, _ := runtime.CallersFrames(pcs[:]).Next()
		if frame.Function == modulePrefix+"sdk.NewError" ||
			(strings.HasSuffix(frame.File, "/errors.go") &&
				(strings.HasPrefix(frame.Function, modulePrefix+"sdk/pixiv.") || strings.HasPrefix(frame.Function, modulePrefix+"sdk/fanbox."))) {
			continue
		}
		return ErrorOrigin{pc: pcs[0]}
	}
}

// OriginAttr 从错误链取得 SDK 的私有位置；不呈现任何底层错误文本。
func OriginAttr(err error) []slog.Attr {
	var located interface{ errorOrigin() string }
	if errors.As(err, &located) {
		if origin := located.errorOrigin(); origin != "" {
			return []slog.Attr{slog.String(AttrOrigin, origin)}
		}
	}
	return nil
}
