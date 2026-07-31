package pixiv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
)

const resourceCacheVersion = 1

// ResourceDownloadCacheState 描述一次成功资源下载如何使用了本地 HTTP 缓存。
type ResourceDownloadCacheState string

const (
	ResourceDownloadCacheMiss        ResourceDownloadCacheState = "miss"
	ResourceDownloadCacheRevalidated ResourceDownloadCacheState = "revalidated"
	ResourceDownloadCacheResumed     ResourceDownloadCacheState = "resumed"
	ResourceDownloadCacheRefreshed   ResourceDownloadCacheState = "refreshed"
)

// ResourceDownloadResult 是 DownloadResource 的成功结果。
type ResourceDownloadResult struct {
	DestinationPath string                     `json:"destination_path"`
	CacheState      ResourceDownloadCacheState `json:"cache_state"`
}

// resourceDownloadLocks 只在同一 Client 内保护同一目标路径。缓存和目标文件都是
// 进程外可见状态，若两个并发下载交叉写入，它们可能把不同版本的实体与 validator
// 错配；因此同一路径必须串行，而不同文件仍可并发。
type resourceDownloadLocks struct {
	mu      sync.Mutex
	entries map[string]*resourceDownloadLock
}

type resourceDownloadLock struct {
	mu   sync.Mutex
	refs int
}

func (l *resourceDownloadLocks) lock(destinationPath string) func() {
	key := filepath.Clean(destinationPath)
	l.mu.Lock()
	if l.entries == nil {
		l.entries = make(map[string]*resourceDownloadLock)
	}
	entry := l.entries[key]
	if entry == nil {
		entry = &resourceDownloadLock{}
		l.entries[key] = entry
	}
	entry.refs++
	l.mu.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		l.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(l.entries, key)
		}
		l.mu.Unlock()
	}
}

type resourceCachePaths struct {
	metadata        string
	partial         string
	partialMetadata string
}

// resourceCachePaths 把 sidecar 放在目标同目录的私有 .pixiv-cache 中。散列避免目标
// 文件名接近平台长度上限时再追加后缀而失败，且 partial 与最终文件仍在同一文件系统，
// 因而可以继续使用原有原子替换语义。
func (c *Client) resourceCachePathsFor(destinationPath string) resourceCachePaths {
	directory := c.resourceCacheDirectory(filepath.Dir(destinationPath))
	digest := sha256.Sum256([]byte(filepath.Clean(destinationPath)))
	key := hex.EncodeToString(digest[:])
	return resourceCachePaths{
		metadata:        filepath.Join(directory, key+".json"),
		partial:         filepath.Join(directory, key+".part"),
		partialMetadata: filepath.Join(directory, key+".part.json"),
	}
}

func (c *Client) resourceCacheDirectory(downloadDirectory string) string {
	if configured := strings.TrimSpace(c.resourceCachePath); configured != "" {
		return configured
	}
	return filepath.Join(downloadDirectory, ".pixiv-cache")
}

type resourceCacheMetadata struct {
	Version      int    `json:"version"`
	URL          string `json:"url"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
}

func resourceCacheMetadataFromHeader(ref ResourceRef, header http.Header) resourceCacheMetadata {
	return resourceCacheMetadata{
		Version:      resourceCacheVersion,
		URL:          ref.URL,
		ETag:         strings.TrimSpace(header.Get("Etag")),
		LastModified: strings.TrimSpace(header.Get("Last-Modified")),
	}
}

func (m resourceCacheMetadata) valid() bool {
	return m.Version == resourceCacheVersion && strings.TrimSpace(m.URL) != "" &&
		!hasControl(m.URL) && !hasControl(m.ETag) && !hasControl(m.LastModified)
}

func (m resourceCacheMetadata) matches(ref ResourceRef) bool {
	return m.URL == ref.URL
}

func (m resourceCacheMetadata) hasValidator() bool {
	return m.ETag != "" || m.LastModified != ""
}

// ifRangeValidator 只在强 ETag 或 Last-Modified 可用时允许续传。弱 ETag 不能证明
// 两个 byte range 属于同一实体，故按 HTTP 语义改用日期；两者都没有时必须重下，
// 而不是拼接未经验证的残片。
func (m resourceCacheMetadata) ifRangeValidator() string {
	if m.ETag != "" && !strings.HasPrefix(strings.ToLower(m.ETag), "w/") {
		return m.ETag
	}
	return m.LastModified
}

func readResourceCacheMetadata(path string) (resourceCacheMetadata, bool, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return resourceCacheMetadata{}, false, nil
	}
	if err != nil {
		return resourceCacheMetadata{}, false, invalidResourceError(OperationDownload, "cannot read download cache metadata")
	}
	var metadata resourceCacheMetadata
	if err := json.Unmarshal(body, &metadata); err != nil || !metadata.valid() {
		return resourceCacheMetadata{}, false, invalidResourceError(OperationDownload, "download cache metadata is invalid")
	}
	return metadata, true, nil
}

func ensureResourceCacheDirectory(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return invalidResourceError(OperationDownload, "cannot create download cache directory")
	}
	return nil
}

func writeResourceCacheMetadata(path string, metadata resourceCacheMetadata) error {
	if !metadata.hasValidator() {
		return removeResourceCacheFile(path)
	}
	if err := ensureResourceCacheDirectory(path); err != nil {
		return err
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		return invalidResourceError(OperationDownload, "cannot encode download cache metadata")
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".pixiv-cache-*")
	if err != nil {
		return invalidResourceError(OperationDownload, "cannot create download cache metadata")
	}
	temporaryPath := temporary.Name()
	keepTemporary := true
	defer func() {
		if keepTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return invalidResourceError(OperationDownload, "cannot protect download cache metadata")
	}
	written, err := temporary.Write(body)
	if err != nil || written != len(body) {
		_ = temporary.Close()
		return invalidResourceError(OperationDownload, "cannot write download cache metadata")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return invalidResourceError(OperationDownload, "cannot sync download cache metadata")
	}
	if err := temporary.Close(); err != nil {
		return invalidResourceError(OperationDownload, "cannot close download cache metadata")
	}
	if err := files.ReplaceFile(temporaryPath, path); err != nil {
		if files.MustPreserveReplacementSource(err) {
			keepTemporary = false
		}
		return invalidResourceError(OperationDownload, "cannot replace download cache metadata")
	}
	keepTemporary = false
	return nil
}

func removeResourceCacheFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return invalidResourceError(OperationDownload, "cannot remove download cache metadata")
	}
	return nil
}

func clearResourcePartial(paths resourceCachePaths) error {
	for _, path := range []string{paths.partial, paths.partialMetadata} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return invalidResourceError(OperationDownload, "cannot clean incomplete download cache")
		}
	}
	return nil
}

// downloadWithCache 对已完成目标使用条件 GET；对已验证残片使用 Range + If-Range。
// 没有安全 validator 的残片从不续接，以免将不同 Pixiv 资源版本拼为一个文件。
func (c *Client) downloadWithCache(ctx context.Context, ref ResourceRef, destinationPath string, replaceFile func(string, string) error, progress func(int64)) (ResourceDownloadCacheState, error) {
	if strings.TrimSpace(destinationPath) == "" {
		return "", invalidResourceError(OperationDownload, "destination path is invalid")
	}
	if err := validateResourceDestinationDirectory(destinationPath); err != nil {
		return "", err
	}
	paths := c.resourceCachePathsFor(destinationPath)

	destinationExists, err := resourceDestinationExists(destinationPath)
	if err != nil {
		return "", err
	}
	if destinationExists {
		metadata, found, err := readResourceCacheMetadata(paths.metadata)
		if err != nil {
			return "", err
		}
		if found && metadata.matches(ref) && metadata.hasValidator() {
			response, err := c.OpenResource(ctx, OpenResourceRequest{
				Ref: ref, IfNoneMatch: metadata.ETag, IfModifiedSince: metadata.LastModified,
			})
			if err != nil {
				return "", resourceErrorForOperation(err, OperationDownload)
			}
			switch response.StatusCode {
			case http.StatusNotModified:
				if err := response.Body.Close(); err != nil {
					return "", newError(CodeUpstreamUnavailable, OperationDownload, BackendResource, true, 0, 0, errors.New("resource stream failed"))
				}
				if err := clearResourcePartial(paths); err != nil {
					return "", err
				}
				return ResourceDownloadCacheRevalidated, nil
			case http.StatusOK:
				return ResourceDownloadCacheRefreshed, c.replaceWithCompleteResource(ctx, response, ref, destinationPath, paths, replaceFile, progress)
			default:
				_ = response.Body.Close()
				return "", incompleteResourceResponseError(response.StatusCode)
			}
		}
	}

	if !destinationExists {
		metadata, found, err := readResourceCacheMetadata(paths.partialMetadata)
		if err != nil {
			return "", err
		}
		partialSize, partialExists, err := resourcePartialSize(paths.partial)
		if err != nil {
			return "", err
		}
		if partialExists && found && !metadata.matches(ref) {
			if err := clearResourcePartial(paths); err != nil {
				return "", err
			}
			partialExists = false
		}
		if partialExists && found && metadata.matches(ref) && metadata.ifRangeValidator() != "" {
			response, err := c.OpenResource(ctx, OpenResourceRequest{
				Ref: ref, Range: fmt.Sprintf("bytes=%d-", partialSize), IfRange: metadata.ifRangeValidator(),
			})
			if err != nil {
				return "", resourceErrorForOperation(err, OperationDownload)
			}
			switch response.StatusCode {
			case http.StatusPartialContent:
				if !contentRangeStartsAt(response.Header.Get("Content-Range"), partialSize) {
					_ = response.Body.Close()
					return "", newError(CodeMalformedUpstreamResponse, OperationDownload, BackendResource, false, response.StatusCode, 0, errors.New("resource partial response has an invalid content range"))
				}
				return ResourceDownloadCacheResumed, c.appendPartialResource(ctx, response, ref, destinationPath, paths, metadata, replaceFile, progress)
			case http.StatusOK:
				// If-Range 不匹配或上游忽略 range 时，200 的完整实体可以安全替换残片。
				return ResourceDownloadCacheRefreshed, c.replaceWithCompleteResource(ctx, response, ref, destinationPath, paths, replaceFile, progress)
			default:
				_ = response.Body.Close()
				return "", incompleteResourceResponseError(response.StatusCode)
			}
		}
	}

	response, err := c.OpenResource(ctx, OpenResourceRequest{Ref: ref})
	if err != nil {
		return "", resourceErrorForOperation(err, OperationDownload)
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return "", incompleteResourceResponseError(response.StatusCode)
	}
	return ResourceDownloadCacheMiss, c.replaceWithCompleteResource(ctx, response, ref, destinationPath, paths, replaceFile, progress)
}

func resourceDestinationExists(destinationPath string) (bool, error) {
	_, err := os.Stat(destinationPath)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, invalidResourceError(OperationDownload, "cannot inspect destination path")
}

func validateResourceDestinationDirectory(destinationPath string) error {
	info, err := os.Stat(filepath.Dir(destinationPath))
	if err != nil || !info.IsDir() {
		return invalidResourceError(OperationDownload, "destination path is invalid")
	}
	return nil
}

func resourcePartialSize(path string) (int64, bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		if err == nil && info.Size() == 0 {
			return 0, false, nil
		}
		return 0, false, invalidResourceError(OperationDownload, "incomplete download cache is invalid")
	}
	return info.Size(), true, nil
}

// verifiedPartialBytes 仅报告与同一资源、且有 If-Range validator 的残片；这样进度
// 初值不会把不安全或过期缓存误算为已完成字节。真实下载路径仍会重新验证并负责报错。
func (c *Client) verifiedPartialBytes(ref ResourceRef, destinationPath string) int64 {
	paths := c.resourceCachePathsFor(destinationPath)
	metadata, found, err := readResourceCacheMetadata(paths.partialMetadata)
	if err != nil || !found || !metadata.matches(ref) || metadata.ifRangeValidator() == "" {
		return 0
	}
	size, exists, err := resourcePartialSize(paths.partial)
	if err != nil || !exists {
		return 0
	}
	return size
}

func (c *Client) replaceWithCompleteResource(ctx context.Context, response *ResourceResponse, ref ResourceRef, destinationPath string, paths resourceCachePaths, replaceFile func(string, string) error, progress func(int64)) error {
	if err := ensureResourceCacheDirectory(paths.partial); err != nil {
		_ = response.Body.Close()
		return err
	}
	file, err := os.OpenFile(paths.partial, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		_ = response.Body.Close()
		return invalidResourceError(OperationDownload, "cannot create incomplete download cache")
	}
	metadata := resourceCacheMetadataFromHeader(ref, response.Header)
	if err := writeResourceCacheMetadata(paths.partialMetadata, metadata); err != nil {
		_ = file.Close()
		_ = response.Body.Close()
		return err
	}
	if err := copyResourceResponse(ctx, response, file, progress); err != nil {
		if !metadata.hasValidator() {
			_ = clearResourcePartial(paths)
		}
		return err
	}
	if err := replaceFile(paths.partial, destinationPath); err != nil {
		if !files.MustPreserveReplacementSource(err) {
			_ = clearResourcePartial(paths)
		}
		return invalidResourceError(OperationDownload, "destination replacement failed")
	}
	if err := writeResourceCacheMetadata(paths.metadata, metadata); err != nil {
		return err
	}
	return clearResourcePartial(paths)
}

func (c *Client) appendPartialResource(ctx context.Context, response *ResourceResponse, ref ResourceRef, destinationPath string, paths resourceCachePaths, metadata resourceCacheMetadata, replaceFile func(string, string) error, progress func(int64)) error {
	file, err := os.OpenFile(paths.partial, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		_ = response.Body.Close()
		return invalidResourceError(OperationDownload, "cannot append incomplete download cache")
	}
	if err := copyResourceResponse(ctx, response, file, progress); err != nil {
		return err
	}
	updated := resourceCacheMetadataFromHeader(ref, response.Header)
	if !updated.hasValidator() {
		updated = metadata
	}
	if err := replaceFile(paths.partial, destinationPath); err != nil {
		if !files.MustPreserveReplacementSource(err) {
			_ = clearResourcePartial(paths)
		}
		return invalidResourceError(OperationDownload, "destination replacement failed")
	}
	if err := writeResourceCacheMetadata(paths.metadata, updated); err != nil {
		return err
	}
	return clearResourcePartial(paths)
}

func copyResourceResponse(ctx context.Context, response *ResourceResponse, destination *os.File, progress func(int64)) error {
	writer := &resourceDestinationWriter{writer: destination, onWrite: progress}
	_, copyErr := io.Copy(writer, response.Body)
	bodyCloseErr := response.Body.Close()
	contextErr := ctx.Err()
	destinationCloseErr := destination.Close()
	if contextErr != nil {
		return mapResourceTransportError(contextErr, OperationDownload)
	}
	if copyErr != nil {
		return classifyResourceCopyError(copyErr, writer.err)
	}
	if bodyCloseErr != nil {
		return newError(CodeUpstreamUnavailable, OperationDownload, BackendResource, true, 0, 0, errors.New("resource stream failed"))
	}
	if destinationCloseErr != nil {
		return invalidResourceError(OperationDownload, "incomplete download cache close failed")
	}
	return nil
}

func contentRangeStartsAt(value string, start int64) bool {
	value = strings.TrimSpace(value)
	if len(value) < len("bytes ") || !strings.EqualFold(value[:len("bytes ")], "bytes ") {
		return false
	}
	rangeValue, _, ok := strings.Cut(strings.TrimSpace(value[len("bytes "):]), "/")
	if !ok {
		return false
	}
	startValue, _, ok := strings.Cut(rangeValue, "-")
	if !ok {
		return false
	}
	got, err := strconv.ParseInt(strings.TrimSpace(startValue), 10, 64)
	return err == nil && got == start
}

func incompleteResourceResponseError(status int) error {
	return newError(CodeUpstreamError, OperationDownload, BackendResource, true, status, 0, errors.New("resource was not a complete response"))
}
