//go:build cgo && ((darwin && (amd64 || arm64)) || (linux && (amd64 || arm64)) || (windows && (amd64 || arm64)))

package ugoira

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

type rustFFI interface {
	NewToken() (unsafe.Pointer, error)
	Cancel(unsafe.Pointer) error
	Free(unsafe.Pointer) error
	Encode(string, []byte, string, unsafe.Pointer, Format, uint32) error
}

type cgoRustFFI struct{}
type rustEncoder struct{ ffi rustFFI }

var rustEncodeGate = make(chan struct{}, 1)

func NewRustEncoder() Encoder { return rustEncoder{ffi: cgoRustFFI{}} }

func (e rustEncoder) Encode(ctx context.Context, input Input) error {
	format, err := input.Format.normalize()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	frames, err := json.Marshal(input.Frames)
	if err != nil {
		return fmt.Errorf("marshal ugoira frames: %w", err)
	}
	ffi := e.ffi
	if ffi == nil {
		ffi = cgoRustFFI{}
	}
	return writeTempAnimation(ctx, input.OutputPath, func(temporaryPath string) error {
		token, err := ffi.NewToken()
		if err != nil {
			return err
		}
		stop := make(chan struct{})
		cancelled := make(chan error, 1)
		go func() {
			select {
			case <-ctx.Done():
				cancelled <- ffi.Cancel(token)
			case <-stop:
				cancelled <- nil
			}
		}()
		select {
		case rustEncodeGate <- struct{}{}:
		case <-ctx.Done():
			close(stop)
			return errors.Join(ctx.Err(), <-cancelled, ffi.Free(token))
		}
		encodeErr := ffi.Encode(input.ZipPath, frames, temporaryPath, token, format, input.MaxEdge)
		<-rustEncodeGate
		close(stop)
		return errors.Join(encodeErr, <-cancelled, ffi.Free(token), ctx.Err())
	})
}

func (cgoRustFFI) NewToken() (unsafe.Pointer, error) {
	token := C.ugoira_cancel_token_new()
	if token == nil {
		return nil, errors.New("create rust ugoira cancellation token failed")
	}
	return unsafe.Pointer(token), nil
}

func (cgoRustFFI) Cancel(token unsafe.Pointer) error {
	return rustError("cancel rust ugoira encoder", C.ugoira_cancel_token_cancel((*C.UgoiraCancellationToken)(token)))
}

func (cgoRustFFI) Free(token unsafe.Pointer) error {
	return rustError("free rust ugoira cancellation token", C.ugoira_cancel_token_free((*C.UgoiraCancellationToken)(token)))
}

func (cgoRustFFI) Encode(zipPath string, frames []byte, outputPath string, token unsafe.Pointer, format Format, maxEdge uint32) error {
	var code C.uint
	switch format {
	case FormatGIF:
		code = 0
	case FormatAPNG:
		code = 1
	}
	zip := C.CString(zipPath)
	defer C.free(unsafe.Pointer(zip))
	jsonBody := C.CString(string(frames))
	defer C.free(unsafe.Pointer(jsonBody))
	out := C.CString(outputPath)
	defer C.free(unsafe.Pointer(out))
	return rustError("rust ugoira encoder failed", C.ugoira_encode(zip, jsonBody, out, (*C.UgoiraCancellationToken)(token), code, C.uint(maxEdge)))
}

func rustError(operation string, pointer *C.char) error {
	if pointer == nil {
		return nil
	}
	message := C.GoString(pointer)
	C.ugoira_free_error(pointer)
	return fmt.Errorf("%s: %s", operation, message)
}
