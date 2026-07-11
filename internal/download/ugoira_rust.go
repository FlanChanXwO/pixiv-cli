//go:build cgo && ((darwin && (amd64 || arm64)) || (linux && (amd64 || arm64)) || (windows && (amd64 || arm64)))

package download

/*
#include <stdlib.h>

typedef struct UgoiraCancellationToken UgoiraCancellationToken;

UgoiraCancellationToken* ugoira_cancel_token_new(void);
char* ugoira_cancel_token_cancel(const UgoiraCancellationToken* token);
char* ugoira_cancel_token_free(UgoiraCancellationToken* token);
char* ugoira_encode(const char* zip_path, const char* frames_json, const char* output_path, const UgoiraCancellationToken* token, unsigned int format, unsigned int max_edge);
void ugoira_free_error(char* err);
*/
import "C"

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"unsafe"
)

type rustUgoiraFFI interface {
	NewToken() (unsafe.Pointer, error)
	Cancel(unsafe.Pointer) error
	Free(unsafe.Pointer) error
	Encode(zipPath string, framesJSON []byte, outputPath string, token unsafe.Pointer, format AnimationFormat, maxEdge uint32) error
}
type cgoRustUgoiraFFI struct{}

type rustUgoiraEncoder struct {
	ffi rustUgoiraFFI
}

// rustUgoiraEncodeGate 只串行化同步 Rust FFI 编码本身；准备、取消清理和文件发布均不占用门禁。
var rustUgoiraEncodeGate = make(chan struct{}, 1)

func withRustUgoiraEncodeGate(ctx context.Context, encode func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case rustUgoiraEncodeGate <- struct{}{}:
		defer func() { <-rustUgoiraEncodeGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	// 取消可能与取得 gate 同时发生；进入同步 FFI 前再检查一次。
	if err := ctx.Err(); err != nil {
		return err
	}
	return encode()
}

func defaultUgoiraEncoder() UgoiraEncoder {
	return NewRustUgoiraEncoder()
}

func NewRustUgoiraEncoder() UgoiraEncoder {
	return rustUgoiraEncoder{ffi: cgoRustUgoiraFFI{}}
}

func (e rustUgoiraEncoder) Encode(ctx context.Context, input UgoiraEncodeInput) error {
	format, err := input.Format.normalize()
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	framesJSON, err := json.Marshal(input.Frames)
	if err != nil {
		return fmt.Errorf("marshal ugoira frames: %w", err)
	}
	ffi := e.ffi
	if ffi == nil {
		ffi = cgoRustUgoiraFFI{}
	}
	return writeTempAnimation(ctx, input.OutputPath, func(tmpOutput string) error {
		token, err := ffi.NewToken()
		if err != nil {
			return err
		}
		stopCancellation := make(chan struct{})
		cancellationDone := make(chan error, 1)
		go func() {
			select {
			case <-ctx.Done():
				cancellationDone <- ffi.Cancel(token)
			case <-stopCancellation:
				cancellationDone <- nil
			}
		}()

		encodeErr := withRustUgoiraEncodeGate(ctx, func() error {
			return ffi.Encode(input.ZipPath, framesJSON, tmpOutput, token, format, input.MaxEdge)
		})
		close(stopCancellation)
		cancelErr := <-cancellationDone
		freeErr := ffi.Free(token)

		return errors.Join(encodeErr, cancelErr, freeErr, ctx.Err())
	})
}

func (cgoRustUgoiraFFI) NewToken() (unsafe.Pointer, error) {
	token := C.ugoira_cancel_token_new()
	if token == nil {
		return nil, fmt.Errorf("create rust ugoira cancellation token: allocation or panic failed")
	}
	return unsafe.Pointer(token), nil
}

func (cgoRustUgoiraFFI) Cancel(token unsafe.Pointer) error {
	return rustFFIError(
		"cancel rust ugoira encoder",
		C.ugoira_cancel_token_cancel((*C.UgoiraCancellationToken)(token)),
	)
}

func (cgoRustUgoiraFFI) Free(token unsafe.Pointer) error {
	return rustFFIError(
		"free rust ugoira cancellation token",
		C.ugoira_cancel_token_free((*C.UgoiraCancellationToken)(token)),
	)
}

func (cgoRustUgoiraFFI) Encode(zipPath string, framesJSON []byte, outputPath string, token unsafe.Pointer, format AnimationFormat, maxEdge uint32) error {
	format, err := format.normalize()
	if err != nil {
		return err
	}
	var formatCode C.uint
	switch format {
	case AnimationFormatGIF:
		formatCode = 0
	case AnimationFormatAPNG:
		formatCode = 1
	}
	cZipPath := C.CString(zipPath)
	defer C.free(unsafe.Pointer(cZipPath))
	cFramesJSON := C.CString(string(framesJSON))
	defer C.free(unsafe.Pointer(cFramesJSON))
	cOutputPath := C.CString(outputPath)
	defer C.free(unsafe.Pointer(cOutputPath))
	return rustFFIError(
		"rust ugoira encoder failed",
		C.ugoira_encode(
			cZipPath,
			cFramesJSON,
			cOutputPath,
			(*C.UgoiraCancellationToken)(token),
			formatCode,
			C.uint(maxEdge),
		),
	)
}

func rustFFIError(operation string, errPtr *C.char) error {
	if errPtr == nil {
		return nil
	}
	message := C.GoString(errPtr)
	C.ugoira_free_error(errPtr)
	return fmt.Errorf("%s: %s", operation, message)
}
