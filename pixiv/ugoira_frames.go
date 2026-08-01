package pixiv

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/ugoira"
)

// extractUgoiraFrames 将 Pixiv 元数据明确列出的帧解压到专用目录。只接受平坦、
// 唯一文件名，既阻断 ZIP traversal，也避免上游 ZIP 中未引用条目落盘。
func extractUgoiraFrames(ctx context.Context, zipPath, outputDirectory string, frames []ugoira.Frame) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open ugoira ZIP: %w", err)
	}
	defer reader.Close()
	entries := make(map[string]*zip.File, len(reader.File))
	for _, entry := range reader.File {
		entries[entry.Name] = entry
	}
	if err := os.MkdirAll(filepath.Dir(outputDirectory), 0o755); err != nil {
		return fmt.Errorf("create ugoira frames parent directory: %w", err)
	}
	temporary, err := os.MkdirTemp(filepath.Dir(outputDirectory), ".ugoira-frames-")
	if err != nil {
		return fmt.Errorf("create ugoira frames directory: %w", err)
	}
	keepTemporary := true
	defer func() {
		if keepTemporary {
			_ = os.RemoveAll(temporary)
		}
	}()
	seen := make(map[string]struct{}, len(frames))
	for _, frame := range frames {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := filepath.Base(frame.File)
		if name == "." || name == string(filepath.Separator) || name != frame.File || strings.Contains(name, `\`) {
			return fmt.Errorf("ugoira metadata contains an unsafe frame filename")
		}
		if _, duplicated := seen[name]; duplicated {
			return fmt.Errorf("ugoira metadata contains a duplicate frame filename")
		}
		seen[name] = struct{}{}
		entry := entries[frame.File]
		if entry == nil || entry.FileInfo().IsDir() {
			return fmt.Errorf("ugoira ZIP does not contain declared frame")
		}
		input, err := entry.Open()
		if err != nil {
			return fmt.Errorf("open ugoira frame: %w", err)
		}
		output, err := os.OpenFile(filepath.Join(temporary, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_, copyErr := io.Copy(output, input)
			closeErr := output.Close()
			if copyErr != nil {
				err = copyErr
			} else {
				err = closeErr
			}
		}
		inputCloseErr := input.Close()
		if err != nil {
			return fmt.Errorf("write ugoira frame: %w", err)
		}
		if inputCloseErr != nil {
			return fmt.Errorf("close ugoira frame: %w", inputCloseErr)
		}
	}
	if err := writeUgoiraFrameManifest(temporary, frames); err != nil {
		return err
	}
	// 先将既有目录移至同一文件系统内的临时备份。发布新目录失败时恢复原目录，
	// 不把一次编码后的替换失败变成用户已有 frames 输出被删除。
	backup := temporary + ".previous"
	if err := os.Rename(outputDirectory, backup); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stage previous ugoira frames directory: %w", err)
	}
	if err := os.Rename(temporary, outputDirectory); err != nil {
		_ = os.Rename(backup, outputDirectory)
		return fmt.Errorf("publish ugoira frames directory: %w", err)
	}
	if err := os.RemoveAll(backup); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clean previous ugoira frames directory: %w", err)
	}
	keepTemporary = false
	return nil
}

// writeUgoiraFrameManifest 在 frames 模式始终写入时间描述；它不是可选 metadata
// sidecar 的替代品，而是将每帧的 Pixiv 延迟与解压后的文件固定对应起来。
func writeUgoiraFrameManifest(directory string, frames []ugoira.Frame) error {
	type frameTiming struct {
		File  string `json:"file"`
		Delay int    `json:"delay"`
	}
	payload := struct {
		Frames []frameTiming `json:"frames"`
	}{Frames: make([]frameTiming, len(frames))}
	for index, frame := range frames {
		payload.Frames[index] = frameTiming{File: frame.File, Delay: frame.Delay}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode ugoira frames manifest: %w", err)
	}
	manifest, err := os.OpenFile(filepath.Join(directory, "frames.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create ugoira frames manifest: %w", err)
	}
	if _, err := manifest.Write(body); err != nil {
		_ = manifest.Close()
		return fmt.Errorf("write ugoira frames manifest: %w", err)
	}
	if err := manifest.Close(); err != nil {
		return fmt.Errorf("close ugoira frames manifest: %w", err)
	}
	return nil
}
