package understandgraph

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type containedFileReader func(root, relativePath, boundaryName string) ([]byte, error)

type resolvedContainedFile struct {
	path string
	info os.FileInfo
}

func readContainedRegularFile(root, relativePath, boundaryName string) ([]byte, error) {
	return readContainedRegularFileWithHook(root, relativePath, boundaryName, nil)
}

func readContainedRegularFileWithHook(root, relativePath, boundaryName string, afterInitialResolution func(string) error) (content []byte, returnErr error) {
	initialFile, err := resolveContainedRegularFile(root, relativePath, boundaryName)
	if err != nil {
		return nil, err
	}
	if afterInitialResolution != nil {
		if err := afterInitialResolution(initialFile.path); err != nil {
			return nil, fmt.Errorf("after initial path resolution: %w", err)
		}
	}
	file, err := openContainedFile(initialFile.path)
	if err != nil {
		return nil, fmt.Errorf("open path %q within %s: %w", relativePath, boundaryName, err)
	}
	defer func() {
		returnErr = errors.Join(returnErr, file.Close())
	}()
	if err := verifyOpenedContainedFile(file, initialFile.info, root, relativePath, boundaryName); err != nil {
		return nil, err
	}
	content, err = io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read opened path %q within %s: %w", relativePath, boundaryName, err)
	}
	// 读取后再核对一次路径与 fd 身份；若读取期间被替换，显式失败且不会产出图谱。
	if err := verifyOpenedContainedFile(file, initialFile.info, root, relativePath, boundaryName); err != nil {
		return nil, err
	}
	return content, nil
}

func verifyOpenedContainedFile(file *os.File, initialInfo os.FileInfo, root, relativePath, boundaryName string) error {
	openedInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat opened path %q within %s: %w", relativePath, boundaryName, err)
	}
	if !openedInfo.Mode().IsRegular() {
		return fmt.Errorf("opened path %q within %s is not a regular file", relativePath, boundaryName)
	}
	currentFile, err := resolveContainedRegularFile(root, relativePath, boundaryName)
	if err != nil {
		return err
	}
	if !sameContainedFile(openedInfo, currentFile.info) {
		return fmt.Errorf("path %q within %s changed during secure read", relativePath, boundaryName)
	}
	if !sameContainedFile(initialInfo, openedInfo) {
		return fmt.Errorf("path %q within %s changed after initial validation", relativePath, boundaryName)
	}
	return nil
}

// sameContainedFile 同时校验文件身份和可观察元数据。Windows 文件系统可能在
// 替换发生后仍给出相同的身份信息；大小、权限或修改时间变化时一律在解析前拒绝，
// 避免这类已观测到的替换把受控源码内容带入错误或图谱产物。
func sameContainedFile(left, right os.FileInfo) bool {
	return os.SameFile(left, right) &&
		left.Mode() == right.Mode() &&
		left.Size() == right.Size() &&
		left.ModTime().Equal(right.ModTime())
}

// resolveContainedRegularFile 校验生成器输入只能指向指定根目录内的普通文件。
// 路径来自可修改的图谱中间产物，因此既要拦截词法越界，也要在解析 symlink 后再次检查真实边界。
func resolveContainedRegularFile(root, relativePath, boundaryName string) (resolvedContainedFile, error) {
	if relativePath == "" {
		return resolvedContainedFile{}, fmt.Errorf("path %q is empty", relativePath)
	}
	localPath := filepath.FromSlash(relativePath)
	cleanPath := filepath.Clean(localPath)
	if filepath.IsAbs(localPath) {
		return resolvedContainedFile{}, fmt.Errorf("path %q is an absolute path outside %s", relativePath, boundaryName)
	}
	if escapesRoot(cleanPath) {
		return resolvedContainedFile{}, fmt.Errorf("path %q is outside %s", relativePath, boundaryName)
	}

	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return resolvedContainedFile{}, fmt.Errorf("resolve %s root: %w", boundaryName, err)
	}
	resolvedRoot, err = filepath.Abs(resolvedRoot)
	if err != nil {
		return resolvedContainedFile{}, fmt.Errorf("resolve absolute %s root: %w", boundaryName, err)
	}
	resolvedPath, err := filepath.EvalSymlinks(filepath.Join(resolvedRoot, cleanPath))
	if err != nil {
		return resolvedContainedFile{}, fmt.Errorf("resolve path %q within %s: %w", relativePath, boundaryName, err)
	}
	resolvedPath, err = filepath.Abs(resolvedPath)
	if err != nil {
		return resolvedContainedFile{}, fmt.Errorf("resolve absolute path %q within %s: %w", relativePath, boundaryName, err)
	}
	resolvedRelativePath, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil {
		return resolvedContainedFile{}, fmt.Errorf("resolve path %q relative to %s: %w", relativePath, boundaryName, err)
	}
	if escapesRoot(resolvedRelativePath) {
		return resolvedContainedFile{}, fmt.Errorf("path %q resolves outside %s", relativePath, boundaryName)
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return resolvedContainedFile{}, fmt.Errorf("stat path %q within %s: %w", relativePath, boundaryName, err)
	}
	if !info.Mode().IsRegular() {
		return resolvedContainedFile{}, fmt.Errorf("path %q within %s is not a regular file", relativePath, boundaryName)
	}
	// Windows 的 os.SameFile 会延迟按路径取得文件 ID。必须在路径仍对应本次
	// 解析结果时触发该读取；否则替换后的路径会被误当成初始文件。
	if !os.SameFile(info, info) {
		return resolvedContainedFile{}, fmt.Errorf("stat path %q within %s cannot establish a stable file identity", relativePath, boundaryName)
	}
	return resolvedContainedFile{path: resolvedPath, info: info}, nil
}

func escapesRoot(path string) bool {
	return path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator))
}
