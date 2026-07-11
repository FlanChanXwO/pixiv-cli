package download

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
)

// AnimationFormat 是 Rust 动图编码器内部支持的输出容器；当前下载命令仍固定 GIF。
type AnimationFormat string

const (
	AnimationFormatGIF  AnimationFormat = "gif"
	AnimationFormatAPNG AnimationFormat = "apng"
)

func (f AnimationFormat) normalize() (AnimationFormat, error) {
	if f == "" {
		return AnimationFormatGIF, nil
	}
	switch f {
	case AnimationFormatGIF, AnimationFormatAPNG:
		return f, nil
	default:
		return "", fmt.Errorf("invalid ugoira animation format %q; expected gif or apng", f)
	}
}

// UgoiraEncoder 在保持 zip 输入与既有命名语义的前提下产生最终动图。
type UgoiraEncoder interface {
	Encode(context.Context, UgoiraEncodeInput) error
}

// UgoiraEncodeInput 是 Go 下载器与 Rust FFI 之间的窄边界。
type UgoiraEncodeInput struct {
	ZipPath    string
	Frames     []pixiv.UgoiraFrame
	WorkDir    string
	OutputPath string
	Format     AnimationFormat
	MaxEdge    uint32
}

func writeTempAnimation(ctx context.Context, outputPath string, encode func(tmpOutput string) error) error {
	extension := filepath.Ext(outputPath)
	if extension == "" {
		return fmt.Errorf("ugoira output path %q has no file extension", outputPath)
	}
	outFile, err := os.CreateTemp(filepath.Dir(outputPath), ".ugoira-*"+extension)
	if err != nil {
		return err
	}
	tmpOutput := outFile.Name()
	if err := outFile.Close(); err != nil {
		_ = os.Remove(tmpOutput)
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpOutput)
		}
	}()
	if err := encode(tmpOutput); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// 动画转换以同目录临时文件发布，编码失败或取消都不会覆盖已有目标。
	if err := files.ReplaceFile(tmpOutput, outputPath); err != nil {
		return err
	}
	cleanup = false
	return nil
}
