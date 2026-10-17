package ugoira

import (
	"bufio"
	"context"
	"errors"
	"os"
)

type encoder struct{}

// 所有实例共用单编码槽，避免并发任务同时分配帧与量化缓冲。
var encodeGate = make(chan struct{}, 1)

// NewEncoder 返回纯 Go GIF/APNG 编码器；所有实例共享串行编码槽。
func NewEncoder() Encoder { return encoder{} }

func (encoder) Encode(ctx context.Context, input Input) error {
	format, err := input.Format.normalize()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case encodeGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-encodeGate }()
	if err := ctx.Err(); err != nil {
		return err
	}
	return writeTempAnimation(ctx, input.OutputPath, func(path string) error {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
		if err != nil {
			return err
		}
		// profile 中默认 4 KiB 缓冲让 APNG 过半 CPU 花在文件写入；合并小 chunk，不缓存整帧。
		output := bufio.NewWriterSize(file, 64*1024)
		if format == FormatGIF {
			err = encodeGIF(ctx, output, input)
		} else {
			err = encodeAPNG(ctx, output, input)
		}
		if err == nil {
			err = output.Flush()
		}
		return errors.Join(err, file.Close(), ctx.Err())
	})
}
