// Package ugoira 提供与 Pixiv 协议无关的 Ugoira ZIP 动图编码能力。
//
// 编码器串行处理同一进程内的任务，并在发布前把输出写入临时文件。
//
// 包的组成：[NewEncoder] 返回 [Encoder]，[Input] 描述源 ZIP、帧延迟、缩放与输出
// 选项，[Format] 选择 GIF 或 APNG；gif.go、apng.go 实现两种容器，frames.go 读取帧。
package ugoira

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/atomicfile"
)

// ErrOutputCapacity 表示编码输出超出了预留的临时空间预算，文件不会被发布。
var ErrOutputCapacity = errors.New("animation temporary capacity exhausted")

// Format 选择输出动画容器。
type Format string

// FormatGIF 输出 GIF，FormatAPNG 输出 APNG；空值由编码器按 GIF 处理。
const (
	FormatGIF  Format = "gif"
	FormatAPNG Format = "apng"
)

func (f Format) normalize() (Format, error) {
	if f == "" {
		return FormatGIF, nil
	}
	switch f {
	case FormatGIF, FormatAPNG:
		return f, nil
	default:
		return "", fmt.Errorf("invalid ugoira animation format %q; expected gif or apng", f)
	}
}

// Frame 保存 ZIP 内一帧的成员名与显示时长，Delay 的单位为毫秒。
type Frame struct {
	File  string `json:"file"`
	Delay int    `json:"delay"`
}

// Input 描述一次编码任务；MaxEdge 为 0 时保持原尺寸。
type Input struct {
	ZipPath    string
	Frames     []Frame
	WorkDir    string
	OutputPath string
	Format     Format
	MaxEdge    uint32
	// MaxOutputBytes 非 nil 时限制发布前的写入字节数。
	MaxOutputBytes *uint64
	// ReserveOutput 在每次同步写文件前申请共享容量；Encode 返回后不再持有该回调，
	// 已申请的部分由 owner 结算。
	ReserveOutput func(uint64) error
	// Observe 接收排队等待与编码耗时，参数不含文件路径。
	Observe func(string, time.Duration)
}

// Encoder 把 Input 编码并原子发布到目标路径；实现串行处理同一进程内的任务。
type Encoder interface {
	Encode(context.Context, Input) error
}

func writeTempAnimation(ctx context.Context, outputPath string, encode func(string) error) (resultErr error) {
	extension := filepath.Ext(outputPath)
	if extension == "" {
		return fmt.Errorf("ugoira output path %q has no file extension", outputPath)
	}
	outFile, err := os.CreateTemp(filepath.Dir(outputPath), ".ugoira-*"+extension)
	if err != nil {
		return err
	}
	temporaryPath := outFile.Name()
	if err := outFile.Close(); err != nil {
		return errors.Join(err, os.Remove(temporaryPath))
	}
	cleanup := true
	defer func() {
		if cleanup {
			if err := os.Remove(temporaryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				resultErr = errors.Join(resultErr, err)
			}
		}
	}()
	if err := encode(temporaryPath); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := atomicfile.ReplaceFile(temporaryPath, outputPath); err != nil {
		if atomicfile.MustPreserveReplacementSource(err) {
			cleanup = false
		}
		return err
	}
	cleanup = false
	return nil
}
