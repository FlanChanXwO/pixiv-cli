package vectormodel

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/eliben/go-sentencepiece"
)

var inferenceGate = make(chan struct{}, 1)

type localModel struct {
	mu           sync.Mutex
	directory    string
	plugin       string
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
		return spec, platform, "", "", i18n.Errorf("error.local_vector_models_are_unsupported_on_s_s_use_a_cloud_model", runtime.GOOS, runtime.GOARCH)
	}
	modelDir := filepath.Join(directory, "models", spec.name+"@"+spec.revision)
	runtimeDir := filepath.Join(directory, "runtime", "onnxruntime-"+runtimeVersion+"-"+platform.platform)
	return spec, platform, modelDir, runtimeDir, nil
}

func openLocal(options Options) (*localModel, error) {
	if options.Device != "" && options.Device != "cpu" && options.Device != "gpu" {
		return nil, i18n.Errorf("error.local_vector_device_must_be_cpu_or_gpu")
	}
	spec, platform, modelDir, runtimeDir, err := localPaths(options.Directory, options.Name)
	if err != nil {
		return nil, err
	}
	for _, dir := range []string{modelDir, runtimeDir} {
		info, err := os.Stat(dir)
		if err != nil {
			return nil, i18n.Errorf("error.local_vector_assets_are_unavailable_run_pixiv_vector_setup_w", err)
		}
		if !info.IsDir() {
			return nil, i18n.Errorf("error.local_vector_asset_directory_is_invalid_run_vector_setup")
		}
	}
	plugin := ""
	if options.Device == "gpu" {
		gpu, dir := gpuPath(options.Directory)
		plugin = filepath.Join(dir, gpu.library)
		if _, err := os.Stat(plugin); err != nil {
			return nil, gpuError(options.Name, err)
		}
	}
	native, err := openNative(filepath.Join(runtimeDir, platform.library))
	if err != nil {
		return nil, err
	}
	return &localModel{directory: modelDir, spec: spec, native: native, plugin: plugin}, nil
}

func (m *localModel) image(ctx context.Context, data []byte) ([]float32, error) {
	select {
	case inferenceGate <- struct{}{}:
		defer func() { <-inferenceGate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, i18n.Errorf("error.local_vector_model_is_closed")
	}
	pixels, err := pixels(ctx, data, m.spec.size)
	if err != nil {
		return nil, err
	}
	if m.vision == 0 {
		m.vision, err = m.native.session(ctx, filepath.Join(m.directory, "onnx", "vision_model.onnx"), m.plugin)
		if err != nil {
			if m.plugin != "" {
				err = gpuError(m.spec.name, err)
			}
			return nil, err
		}
	}
	vector, err := m.native.infer(ctx, m.vision, "pixel_values", unsafe.Pointer(&pixels[0]), len(pixels)*4, []int64{1, 3, int64(m.spec.size), int64(m.spec.size)}, nativeFloat)
	if err != nil && m.plugin != "" && ctx.Err() == nil {
		err = gpuError(m.spec.name, err)
	}
	return vector, err
}

func (m *localModel) textVector(ctx context.Context, text string) ([]float32, error) {
	select {
	case inferenceGate <- struct{}{}:
		defer func() { <-inferenceGate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, i18n.Errorf("error.local_vector_model_is_closed")
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
		return nil, i18n.Errorf("error.w_maximum_d_tokens_including_eos", ErrTextTooLong, textLength)
	}
	ids := make([]int64, textLength)
	for i, t := range tokens {
		ids[i] = int64(t.ID)
	}
	ids[len(tokens)] = textEOS
	if m.text == 0 {
		m.text, err = m.native.session(ctx, filepath.Join(m.directory, "onnx", "text_model.onnx"), "")
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

func gpuPath(directory string) (nativeSpec, string) {
	spec := gpuPlatforms[runtime.GOOS+"/"+runtime.GOARCH]
	return spec, filepath.Join(directory, "runtime", "webgpu-"+gpuVersion+"-"+spec.platform)
}

func gpuError(name string, err error) error {
	return i18n.Errorf("error.gpu_unavailable_run_pixiv_vector_setup_model_s_device_cpu_w", name, err)
}
