// Package ugoira 提供与 Pixiv 协议无关的 Ugoira ZIP 动图编码能力。
package ugoira

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/internal/filesystem"
)

type Format string

const (
	FormatGIF  Format = "gif"
	FormatAPNG Format = "apng"
)

func (f Format) normalize() (Format, error) {
	if f == "" {
		return FormatGIF, nil
	}
	switch f {
	case FormatGIF, FormatAPNG:
		return f, nil
	default:
		return "", fmt.Errorf("invalid ugoira animation format %q; expected gif or apng", f)
	}
}

type Frame struct {
	File  string `json:"file"`
	Delay int    `json:"delay"`
}

type Input struct {
	ZipPath    string
	Frames     []Frame
	WorkDir    string
	OutputPath string
	Format     Format
	MaxEdge    uint32
}

type Encoder interface {
	Encode(context.Context, Input) error
}

func writeTempAnimation(ctx context.Context, outputPath string, encode func(string) error) error {
	extension := filepath.Ext(outputPath)
	if extension == "" {
		return fmt.Errorf("ugoira output path %q has no file extension", outputPath)
	}
	outFile, err := os.CreateTemp(filepath.Dir(outputPath), ".ugoira-*"+extension)
	if err != nil {
		return err
	}
	temporaryPath := outFile.Name()
	if err := outFile.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := encode(temporaryPath); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := filesystem.ReplaceFile(temporaryPath, outputPath); err != nil {
		if filesystem.MustPreserveReplacementSource(err) {
			cleanup = false
		}
		return err
	}
	cleanup = false
	return nil
}
