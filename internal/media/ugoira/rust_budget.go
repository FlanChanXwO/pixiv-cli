//go:build cgo && ((darwin && (amd64 || arm64)) || (linux && (amd64 || arm64)) || (windows && (amd64 || arm64)))

package ugoira

/*
#include <stdlib.h>
typedef struct UgoiraCancellationToken UgoiraCancellationToken;
typedef int (*UgoiraOutputReserve)(unsigned long long, unsigned long long);
extern int reserveUgoiraOutput(unsigned long long handle, unsigned long long bytes);
char* ugoira_encode_budgeted(const char*, const char*, const char*, const UgoiraCancellationToken*, unsigned int, unsigned int, unsigned long long, UgoiraOutputReserve, unsigned long long);
*/
import "C"

import (
	"errors"
	"runtime/cgo"
	"sync"
	"unsafe"
)

type nativeOutputBudget struct {
	mu      sync.Mutex
	reserve func(uint64) error
	err     error
}

//export reserveUgoiraOutput
func reserveUgoiraOutput(handle, bytes C.ulonglong) (allowed C.int) {
	var budget *nativeOutputBudget
	defer func() {
		if recover() != nil {
			if budget != nil {
				budget.mu.Lock()
				budget.err = errors.New("native output budget callback panicked")
				budget.mu.Unlock()
			}
			allowed = 0
		}
	}()
	budget = cgo.Handle(handle).Value().(*nativeOutputBudget)
	budget.mu.Lock()
	defer budget.mu.Unlock()
	if budget.err == nil {
		budget.err = budget.reserve(uint64(bytes))
	}
	if budget.err == nil {
		return 1
	}
	return 0
}

func (cgoRustFFI) EncodeBudgeted(zipPath string, frames []byte, outputPath string, token unsafe.Pointer, format Format, maxEdge uint32, limit uint64, reserve func(uint64) error) error {
	var code C.uint
	if format == FormatAPNG {
		code = 1
	}
	zip := C.CString(zipPath)
	defer C.free(unsafe.Pointer(zip))
	frameJSON := C.CString(string(frames))
	defer C.free(unsafe.Pointer(frameJSON))
	out := C.CString(outputPath)
	defer C.free(unsafe.Pointer(out))
	budget := &nativeOutputBudget{reserve: reserve}
	handle := cgo.NewHandle(budget)
	// The callback is synchronous; delete only after Rust has dropped all writers.
	defer handle.Delete()
	err := rustError("rust ugoira encoder failed", C.ugoira_encode_budgeted(zip, frameJSON, out, (*C.UgoiraCancellationToken)(token), code, C.uint(maxEdge), C.ulonglong(limit), C.UgoiraOutputReserve(C.reserveUgoiraOutput), C.ulonglong(handle)))
	budget.mu.Lock()
	defer budget.mu.Unlock()
	if budget.err != nil {
		return budget.err
	}
	return err
}
