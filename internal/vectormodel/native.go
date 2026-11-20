package vectormodel

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

const (
	nativeAPIVersion     = 32
	nativeErrorSeverity  = 3
	nativeArenaAllocator = 1
	nativeDefaultMemory  = 0
	nativeFloat          = 1
	nativeInt64          = 7
)

// ONNX Runtime 1.32 的 OrtApi 槽位；升级运行库时与同版本 C header 一起核对。
const (
	apiErrorMessage   = 2
	apiCreateEnv      = 3
	apiCreateSession  = 7
	apiRun            = 9
	apiCreateOptions  = 10
	apiCreateRun      = 39
	apiTerminate      = 46
	apiCreateTensor   = 49
	apiData           = 51
	apiCount          = 64
	apiShape          = 65
	apiCreateMemory   = 69
	apiReleaseEnv     = 92
	apiReleaseStatus  = 93
	apiReleaseMemory  = 94
	apiReleaseSession = 95
	apiReleaseValue   = 96
	apiReleaseRun     = 97
	apiReleaseShape   = 99
	apiReleaseOptions = 100
	apiRegisterEP     = 301
	apiGetDevices     = 303
	apiAppendDevices  = 304
	apiHardwareType   = 307
	apiEPName         = 312
	apiEPDevice       = 316
)

type nativeRuntime struct {
	env                                                                                                              uintptr
	errorMessage                                                                                                     func(uintptr) string
	releaseStatus, releaseEnv, releaseOptions, releaseSession, releaseMemory, releaseValue, releaseShape, releaseRun func(uintptr)
	createEnv                                                                                                        func(int32, string, *uintptr) uintptr
	createOptions, createRun                                                                                         func(*uintptr) uintptr
	createSession                                                                                                    func(uintptr, uintptr, uintptr, *uintptr) uintptr
	createMemory                                                                                                     func(int32, int32, *uintptr) uintptr
	createTensor                                                                                                     func(uintptr, uintptr, uintptr, uintptr, uintptr, int32, *uintptr) uintptr
	run                                                                                                              func(uintptr, uintptr, uintptr, uintptr, uintptr, uintptr, uintptr, *uintptr) uintptr
	data                                                                                                             func(uintptr, *unsafe.Pointer) uintptr
	shape, count                                                                                                     func(uintptr, *uintptr) uintptr
	registerEP                                                                                                       func(uintptr, string, uintptr) uintptr
	getDevices                                                                                                       func(uintptr, *unsafe.Pointer, *uintptr) uintptr
	appendDevices                                                                                                    func(uintptr, uintptr, uintptr, uintptr, uintptr, uintptr, uintptr) uintptr
	hardwareType                                                                                                     func(uintptr) int32
	epName                                                                                                           func(uintptr) string
	epDevice                                                                                                         func(uintptr) uintptr
	terminate                                                                                                        func(uintptr) uintptr
}

// shortcut: 原生推理与调用方同进程；出现原生崩溃报告后再隔离到子进程。
func openNative(path string) (*nativeRuntime, error) {
	symbol, err := loadNative(path)
	if err != nil {
		return nil, fmt.Errorf("load ONNX Runtime (run vector setup): %w", err)
	}
	var baseFunc func() unsafe.Pointer
	purego.RegisterFunc(&baseFunc, symbol)
	base := baseFunc()
	var version func() string
	purego.RegisterFunc(&version, *(*uintptr)(unsafe.Add(base, unsafe.Sizeof(uintptr(0)))))
	if got := version(); got != runtimeVersion {
		return nil, fmt.Errorf("ONNX Runtime version %s; expected %s", got, runtimeVersion)
	}
	var apiFunc func(uint32) unsafe.Pointer
	purego.RegisterFunc(&apiFunc, *(*uintptr)(base))
	api := apiFunc(nativeAPIVersion)
	if api == nil {
		return nil, errors.New("ONNX Runtime API version unavailable")
	}
	n := &nativeRuntime{}
	bind := func(target any, index uintptr) {
		purego.RegisterFunc(target, *(*uintptr)(unsafe.Add(api, index*unsafe.Sizeof(uintptr(0)))))
	}
	bind(&n.errorMessage, apiErrorMessage)
	bind(&n.registerEP, apiRegisterEP)
	bind(&n.getDevices, apiGetDevices)
	bind(&n.appendDevices, apiAppendDevices)
	bind(&n.hardwareType, apiHardwareType)
	bind(&n.epName, apiEPName)
	bind(&n.epDevice, apiEPDevice)
	bind(&n.createEnv, apiCreateEnv)
	bind(&n.createSession, apiCreateSession)
	bind(&n.run, apiRun)
	bind(&n.createOptions, apiCreateOptions)
	bind(&n.createRun, apiCreateRun)
	bind(&n.terminate, apiTerminate)
	bind(&n.createTensor, apiCreateTensor)
	bind(&n.data, apiData)
	bind(&n.count, apiCount)
	bind(&n.shape, apiShape)
	bind(&n.createMemory, apiCreateMemory)
	bind(&n.releaseEnv, apiReleaseEnv)
	bind(&n.releaseStatus, apiReleaseStatus)
	bind(&n.releaseMemory, apiReleaseMemory)
	bind(&n.releaseSession, apiReleaseSession)
	bind(&n.releaseValue, apiReleaseValue)
	bind(&n.releaseRun, apiReleaseRun)
	bind(&n.releaseShape, apiReleaseShape)
	bind(&n.releaseOptions, apiReleaseOptions)
	if err = n.check(n.createEnv(nativeErrorSeverity, "pixiv-vector", &n.env)); err != nil {
		return nil, err
	}
	return n, nil
}

func (n *nativeRuntime) check(status uintptr) error {
	if status == 0 {
		return nil
	}
	message := n.errorMessage(status)
	n.releaseStatus(status)
	return fmt.Errorf("ONNX Runtime: %s", message)
}

func (n *nativeRuntime) session(ctx context.Context, path, plugin string) (uintptr, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	var options uintptr
	if err := n.check(n.createOptions(&options)); err != nil {
		return 0, err
	}
	defer n.releaseOptions(options)
	if plugin != "" {
		if err := n.gpuOptions(options, plugin); err != nil {
			return 0, err
		}
	}
	encoded, unpin, err := nativePath(path)
	if err != nil {
		return 0, err
	}
	defer unpin()
	var session uintptr
	if err = n.check(n.createSession(n.env, uintptr(encoded), options, &session)); err != nil {
		return 0, err
	}
	if err = ctx.Err(); err != nil {
		n.releaseSession(session)
		return 0, err
	}
	return session, nil
}

func (n *nativeRuntime) infer(ctx context.Context, session uintptr, inputName string, data unsafe.Pointer, byteSize int, shape []int64, element int32) ([]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var memory, input, run uintptr
	if err := n.check(n.createMemory(nativeArenaAllocator, nativeDefaultMemory, &memory)); err != nil {
		return nil, err
	}
	defer n.releaseMemory(memory)
	var pin runtime.Pinner
	pin.Pin(data)
	pin.Pin(&shape[0])
	defer pin.Unpin()
	if err := n.check(n.createTensor(memory, uintptr(data), uintptr(byteSize), uintptr(unsafe.Pointer(&shape[0])), uintptr(len(shape)), element, &input)); err != nil {
		return nil, err
	}
	defer n.releaseValue(input)
	if err := n.check(n.createRun(&run)); err != nil {
		return nil, err
	}
	defer n.releaseRun(run)
	inName, outName := append([]byte(inputName), 0), append([]byte("pooler_output"), 0)
	pin.Pin(&inName[0])
	pin.Pin(&outName[0])
	inNames, outNames := uintptr(unsafe.Pointer(&inName[0])), uintptr(unsafe.Pointer(&outName[0]))
	pin.Pin(&inNames)
	pin.Pin(&outNames)
	pin.Pin(&input)
	finished, joined := make(chan struct{}), make(chan struct{})
	var terminateErr error
	go func() {
		defer close(joined)
		select {
		case <-ctx.Done():
			terminateErr = n.check(n.terminate(run))
		case <-finished:
		}
	}()
	var output uintptr
	err := n.check(n.run(session, run, uintptr(unsafe.Pointer(&inNames)), uintptr(unsafe.Pointer(&input)), 1, uintptr(unsafe.Pointer(&outNames)), 1, &output))
	close(finished)
	<-joined
	if output != 0 {
		defer n.releaseValue(output)
	}
	if ctx.Err() != nil {
		return nil, errors.Join(ctx.Err(), terminateErr)
	}
	if err != nil {
		return nil, err
	}
	var pointer unsafe.Pointer
	var info, count uintptr
	if err = n.check(n.data(output, &pointer)); err != nil {
		return nil, err
	}
	if err = n.check(n.shape(output, &info)); err != nil {
		return nil, err
	}
	defer n.releaseShape(info)
	if err = n.check(n.count(info, &count)); err != nil {
		return nil, err
	}
	if count != localDimensions || pointer == nil {
		return nil, errors.New("unexpected local model output dimensions")
	}
	return normalize(append([]float32(nil), unsafe.Slice((*float32)(pointer), int(count))...))
}

func (n *nativeRuntime) gpuOptions(options uintptr, path string) error {
	encoded, unpin, err := nativePath(path)
	if err != nil {
		return err
	}
	defer unpin()
	if err = n.check(n.registerEP(n.env, "WebGPU", uintptr(encoded))); err != nil {
		return err
	}
	var devices unsafe.Pointer
	var count uintptr
	if err = n.check(n.getDevices(n.env, &devices, &count)); err != nil {
		return err
	}
	var selected uintptr
	for _, device := range unsafe.Slice((*uintptr)(devices), int(count)) {
		if n.epName(device) == "WebGpuExecutionProvider" && n.hardwareType(n.epDevice(device)) == 1 {
			selected = device
			break
		}
	}
	if selected == 0 {
		return errors.New("WebGPU GPU device not found")
	}
	var pin runtime.Pinner
	pin.Pin(&selected)
	defer pin.Unpin()
	return n.check(n.appendDevices(options, n.env, uintptr(unsafe.Pointer(&selected)), 1, 0, 0, 0))
}
