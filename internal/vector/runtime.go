package vector

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

const (
	ModelID    = "google/siglip2-base-patch16-512"
	Generation = "a89f5c5093f902bf39d3cd4d81d2c09867f0724b"
)

//go:embed siglip2.py
var siglip2Script string

type SigLIP2 struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	encode *json.Encoder
	decode *json.Decoder
}

// StartSigLIP2 starts one transient local process. It never downloads model weights.
func StartSigLIP2(ctx context.Context) (*SigLIP2, error) {
	python := os.Getenv("PIXIV_VECTOR_PYTHON")
	if python == "" {
		python = "python3"
	}
	cmd := exec.CommandContext(ctx, python, "-I", "-u", "-c", siglip2Script, ModelID, Generation)
	cmd.Stderr = io.Discard // Python tracebacks can contain private local image paths.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("vector: prepare embedding runtime: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("vector: prepare embedding runtime: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("vector: embedding runtime unavailable: %w", err)
	}
	runtime := &SigLIP2{cmd: cmd, stdin: stdin, encode: json.NewEncoder(stdin), decode: json.NewDecoder(stdout)}
	var ready struct {
		Ready        bool   `json:"ready"`
		StartupError string `json:"startup_error"`
		Detail       string `json:"detail"`
	}
	if err := runtime.decode.Decode(&ready); err != nil || !ready.Ready {
		_ = runtime.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			return nil, fmt.Errorf("vector: embedding runtime stopped before it was ready: %w", err)
		}
		// 结构化 startup error：错误类型可诊断，detail 不含 token 或本地路径。
		switch ready.StartupError {
		case "dependency_missing":
			return nil, fmt.Errorf("vector: embedding runtime dependency %s is missing (requires Python torch, transformers, Pillow)", ready.Detail)
		case "dependency_version_mismatch":
			return nil, fmt.Errorf("vector: embedding runtime dependency version mismatch: %s", ready.Detail)
		case "model_not_found":
			return nil, fmt.Errorf("vector: embedding model not found locally: %s", ready.Detail)
		case "model_load_failed":
			return nil, fmt.Errorf("vector: embedding model failed to load (%s)", ready.Detail)
		}
		return nil, errors.New("vector: embedding runtime unavailable (requires preloaded SigLIP2 weights and Python torch, transformers, Pillow)")
	}
	return runtime, nil
}

func (r *SigLIP2) Image(ctx context.Context, path string) ([]float32, error) {
	return r.embed(ctx, map[string]string{"image": path})
}

func (r *SigLIP2) Text(ctx context.Context, text string) ([]float32, error) {
	return r.embed(ctx, map[string]string{"text": text})
}

func (r *SigLIP2) embed(ctx context.Context, request map[string]string) ([]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := r.encode.Encode(request); err != nil {
		return nil, fmt.Errorf("vector: send request to embedding runtime: %w", err)
	}
	var result struct {
		Vector []float32 `json:"vector"`
		Error  string    `json:"error"`
	}
	if err := r.decode.Decode(&result); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("vector: embedding runtime stopped: %w", err)
	}
	if result.Error != "" {
		if result.Error == "cannot_read_image" {
			return nil, errors.New("vector: embedding runtime cannot read image")
		}
		if result.Error == "cannot_encode_text" {
			return nil, errors.New("vector: embedding runtime cannot encode text")
		}
		return nil, errors.New("vector: embedding runtime failed")
	}
	if len(result.Vector) != 768 { // Pinned SigLIP2 Base 512 projection width, verified with the real model.
		return nil, errors.New("vector: embedding runtime returned an unexpected vector dimension")
	}
	return result.Vector, nil
}

func (r *SigLIP2) Close() error {
	if r == nil {
		return nil
	}
	return errors.Join(r.stdin.Close(), r.cmd.Wait())
}
