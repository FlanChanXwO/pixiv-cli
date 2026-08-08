package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	fileutil "github.com/FlanChanXwO/pixiv-cli/internal/filesystem"
)

type jsonObjectFile struct {
	Path   string
	Object map[string]json.RawMessage
}

type stagedJSONFile struct {
	path     string
	tempPath string
	body     []byte
	mode     os.FileMode
}

func readJSONObject(path string) (map[string]json.RawMessage, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if object == nil {
		return nil, fmt.Errorf("parse %s: root must be an object", path)
	}
	return object, nil
}

// decodeField 只解码当前步骤需要的字段，使 generator 的其他顶层扩展字段保持原始 JSON 语义。
func decodeField[T any](object map[string]json.RawMessage, field string) (T, error) {
	var value T
	raw, ok := object[field]
	if !ok {
		return value, fmt.Errorf("missing required field %q", field)
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, fmt.Errorf("parse field %q: %w", field, err)
	}
	return value, nil
}

func setField(object map[string]json.RawMessage, field string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode field %q: %w", field, err)
	}
	object[field] = raw
	return nil
}

func writeJSONObjects(files ...jsonObjectFile) error {
	return writeJSONObjectsWithReplacer(fileutil.ReplaceFile, files...)
}

func writeJSONObjectsWithReplacer(replace func(string, string) error, files ...jsonObjectFile) error {
	staged := make([]stagedJSONFile, 0, len(files))
	// 先完成所有编码，避免后一个对象不可编码时前一个产物已经被替换。
	for _, file := range files {
		body, err := json.MarshalIndent(file.Object, "", "  ")
		if err != nil {
			return fmt.Errorf("encode %s: %w", file.Path, err)
		}
		mode := os.FileMode(0o600)
		info, err := os.Stat(file.Path)
		if err == nil {
			mode = info.Mode().Perm()
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat %s: %w", file.Path, err)
		}
		staged = append(staged, stagedJSONFile{path: file.Path, body: append(body, '\n'), mode: mode})
	}

	for index := range staged {
		file := &staged[index]
		temporary, err := os.CreateTemp(filepath.Dir(file.path), "."+filepath.Base(file.path)+".tmp-*")
		if err != nil {
			return errors.Join(fmt.Errorf("create temporary file for %s: %w", file.path, err), cleanupStagedJSON(staged))
		}
		file.tempPath = temporary.Name()
		if err := temporary.Chmod(file.mode); err != nil {
			_ = temporary.Close()
			return errors.Join(fmt.Errorf("chmod temporary file for %s: %w", file.path, err), cleanupStagedJSON(staged))
		}
		if _, err := temporary.Write(file.body); err != nil {
			_ = temporary.Close()
			return errors.Join(fmt.Errorf("write temporary file for %s: %w", file.path, err), cleanupStagedJSON(staged))
		}
		if err := temporary.Sync(); err != nil {
			_ = temporary.Close()
			return errors.Join(fmt.Errorf("sync temporary file for %s: %w", file.path, err), cleanupStagedJSON(staged))
		}
		if err := temporary.Close(); err != nil {
			return errors.Join(fmt.Errorf("close temporary file for %s: %w", file.path, err), cleanupStagedJSON(staged))
		}
	}

	// 标准库没有跨多个路径的事务；同目录 rename 保证每份文件自身不会暴露半写内容。
	for index := range staged {
		file := &staged[index]
		if err := replace(file.tempPath, file.path); err != nil {
			cleanupFrom := index
			if fileutil.MustPreserveReplacementSource(err) {
				cleanupFrom = index + 1
			}
			return errors.Join(
				fmt.Errorf("replace %s after %d successful replacements: %w", file.path, index, err),
				cleanupStagedJSON(staged[cleanupFrom:]),
			)
		}
		file.tempPath = ""
	}
	return nil
}

func cleanupStagedJSON(files []stagedJSONFile) error {
	var cleanupErrors []error
	for _, file := range files {
		if file.tempPath == "" {
			continue
		}
		if err := os.Remove(file.tempPath); err != nil && !os.IsNotExist(err) {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove temporary file %s: %w", file.tempPath, err))
		}
		recoveryPath := file.tempPath + ".recovery"
		if err := os.Remove(recoveryPath); err != nil && !os.IsNotExist(err) {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove replacement recovery file %s: %w", recoveryPath, err))
		}
	}
	return errors.Join(cleanupErrors...)
}
