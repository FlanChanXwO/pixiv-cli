package ugoira

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"time"
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
	waiting := time.Now()
	select {
	case encodeGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-encodeGate }()
	if input.Observe != nil {
		input.Observe("animation_encode_wait", time.Since(waiting))
	}
	started := time.Now()
	if input.Observe != nil {
		defer func() { input.Observe("animation_encode", time.Since(started)) }()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return writeTempAnimation(ctx, input.OutputPath, func(path string) error {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
		if err != nil {
			return err
		}
		// profile 中默认 4 KiB 缓冲让 APNG 过半 CPU 花在文件写入；合并小 chunk，不缓存整帧。
		var destination io.Writer = file
		if input.MaxOutputBytes != nil || input.ReserveOutput != nil {
			limit := ^uint64(0)
			if input.MaxOutputBytes != nil {
				limit = *input.MaxOutputBytes
			}
			destination = &outputBudgetWriter{writer: file, remaining: limit, reserve: input.ReserveOutput}
		}
		output := bufio.NewWriterSize(destination, 64*1024)
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

func writeAnimation(ctx context.Context, output io.Writer, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	n, err := output.Write(data)
	if n != len(data) && err == nil {
		err = io.ErrShortWrite
	}
	return errors.Join(err, ctx.Err())
}

// 预算位于缓冲器下游，在每次实际落盘前预留容量；普通调用不受限。
type outputBudgetWriter struct {
	writer    io.Writer
	remaining uint64
	reserve   func(uint64) error
}

func (w *outputBudgetWriter) Write(data []byte) (int, error) {
	if uint64(len(data)) > w.remaining {
		return 0, ErrOutputCapacity
	}
	if w.reserve != nil {
		if err := reserveOutput(w.reserve, uint64(len(data))); err != nil {
			return 0, err
		}
	}
	n, err := w.writer.Write(data)
	w.remaining -= uint64(n)
	return n, err
}

func reserveOutput(reserve func(uint64) error, size uint64) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("animation output budget callback panicked")
		}
	}()
	return reserve(size)
}
