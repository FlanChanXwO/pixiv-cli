package vectormodel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/eliben/go-sentencepiece"
)

var textGate = make(chan struct{}, 1)

type localModel struct {
	mu           sync.Mutex
	directory    string
	spec         modelSpec
	native       *nativeRuntime
	vision, text uintptr
	tokenizer    *sentencepiece.Processor
	closed       bool
}

// IsLocal 识别固定 revision 的内置多模态模型。
func IsLocal(name string) bool { _, ok := localModels[name]; return ok }

func localPaths(directory, name string) (modelSpec, nativeSpec, string, string, error) {
	spec := localModels[name]
	platform, ok := nativePlatforms[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok {
		return spec, platform, "", "", fmt.Errorf("local vector models are unsupported on %s/%s; use a cloud model", runtime.GOOS, runtime.GOARCH)
	}
	modelDir := filepath.Join(directory, "models", spec.name+"@"+spec.revision)
	runtimeDir := filepath.Join(directory, "runtime", "onnxruntime-"+runtimeVersion+"-"+platform.platform)
	return spec, platform, modelDir, runtimeDir, nil
}

func openLocal(options Options) (*localModel, error) {
	if options.Device != "" && options.Device != "cpu" {
		return nil, errors.New("GPU models are not available yet; use vector setup with CPU")
	}
	spec, platform, modelDir, runtimeDir, err := localPaths(options.Directory, options.Name)
	if err != nil {
		return nil, err
	}
	for _, dir := range []string{modelDir, runtimeDir} {
		info, err := os.Stat(dir)
		if err != nil {
			return nil, fmt.Errorf("local vector assets are unavailable; run \"pixiv vector setup\": %w", err)
		}
		if !info.IsDir() {
			return nil, errors.New("local vector asset directory is invalid; run vector setup")
		}
	}
	native, err := openNative(filepath.Join(runtimeDir, platform.library))
	if err != nil {
		return nil, err
	}
	return &localModel{directory: modelDir, spec: spec, native: native}, nil
}

func (m *localModel) image(ctx context.Context, data []byte) ([]float32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, errors.New("local vector model is closed")
	}
	pixels, err := pixels(ctx, data, m.spec.size)
	if err != nil {
		return nil, err
	}
	if m.vision == 0 {
		m.vision, err = m.native.session(ctx, filepath.Join(m.directory, "onnx", "vision_model.onnx"))
		if err != nil {
			return nil, err
		}
	}
	return m.native.infer(ctx, m.vision, "pixel_values", unsafe.Pointer(&pixels[0]), len(pixels)*4, []int64{1, 3, int64(m.spec.size), int64(m.spec.size)}, nativeFloat)
}

func (m *localModel) textVector(ctx context.Context, text string) ([]float32, error) {
	select {
	case textGate <- struct{}{}:
		defer func() { <-textGate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, errors.New("local vector model is closed")
	}
	var err error
	if m.tokenizer == nil {
		m.tokenizer, err = sentencepiece.NewProcessorFromPath(filepath.Join(m.directory, "tokenizer.model"))
		if err != nil {
			return nil, err
		}
	}
	tokens := m.tokenizer.Encode(strings.ToLower(text))
	if len(tokens)+1 > textLength {
		return nil, fmt.Errorf("text query exceeds %d tokens including EOS", textLength)
	}
	ids := make([]int64, textLength)
	for i, t := range tokens {
		ids[i] = int64(t.ID)
	}
	ids[len(tokens)] = textEOS
	if m.text == 0 {
		m.text, err = m.native.session(ctx, filepath.Join(m.directory, "onnx", "text_model.onnx"))
		if err != nil {
			return nil, err
		}
	}
	return m.native.infer(ctx, m.text, "input_ids", unsafe.Pointer(&ids[0]), len(ids)*8, []int64{1, textLength}, nativeInt64)
}

func (m *localModel) close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	if m.vision != 0 {
		m.native.releaseSession(m.vision)
	}
	if m.text != 0 {
		m.native.releaseSession(m.text)
	}
	m.native.releaseEnv(m.native.env)
	return nil
}
