package vectormodel

import (
	"archive/zip"
	"compress/flate"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Install 下载当前平台的运行库及固定模型；校验完成后才发布最终目录。
func Install(ctx context.Context, options Options, client *http.Client, progress func(string) error) error {
	spec, platform, modelDir, runtimeDir, err := localPaths(options.Directory, options.Name)
	if err != nil {
		return err
	}
	if !IsLocal(options.Name) {
		return errors.New("install requires a built-in local model")
	}
	if err = installGroup(ctx, client, runtimeDir, platform.assets, progress); err != nil {
		return err
	}
	if options.Device == "gpu" {
		gpu, dir := gpuPath(options.Directory)
		if err = installGroup(ctx, client, dir, gpu.assets, progress); err != nil {
			return err
		}
	}
	return installGroup(ctx, client, modelDir, spec.assets, progress)
}

func installGroup(ctx context.Context, client *http.Client, directory string, assets []asset, progress func(string) error) error {
	if info, err := os.Stat(directory); err == nil {
		if !info.IsDir() {
			return errors.New("vector asset path is not a directory")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	partial := directory + ".partial"
	if err := os.MkdirAll(partial, 0700); err != nil {
		return err
	}
	archives := map[string]*zip.Reader{}
	sources := map[string]*rangeArchive{}
	for _, a := range assets {
		if err := ctx.Err(); err != nil {
			return err
		}
		target := filepath.Join(partial, filepath.FromSlash(a.path))
		if _, err := os.Stat(target); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if progress != nil {
			if err := progress("Downloading " + a.path); err != nil {
				return err
			}
		}
		source, member, zipped := strings.Cut(a.url, "#")
		if !zipped {
			if err := downloadRange(ctx, client, source, target+".part", 0, a.size, a.size); err != nil {
				return err
			}
			if err := verifyFile(ctx, target+".part", target, a); err != nil {
				return err
			}
			continue
		}
		archive := archives[source]
		if archive == nil {
			remote, err := openArchive(ctx, client, source)
			if err != nil {
				return err
			}
			archive, err = zip.NewReader(remote, remote.size)
			if err != nil {
				return err
			}
			archives[source], sources[source] = archive, remote
		}
		var entry *zip.File
		for _, f := range archive.File {
			if f.Name == member {
				entry = f
				break
			}
		}
		if entry == nil {
			return fmt.Errorf("native package is missing %s", member)
		}
		if entry.UncompressedSize64 != uint64(a.size) {
			return errors.New("native package asset size changed")
		}
		offset, err := entry.DataOffset()
		if err != nil {
			return err
		}
		compressed := target + ".part"
		if err = downloadRange(ctx, client, source, compressed, offset, int64(entry.CompressedSize64), sources[source].size); err != nil {
			return err
		}
		if err = unpackFile(ctx, compressed, target, a, entry.Method); err != nil {
			return err
		}
		if err = os.Remove(compressed); err != nil {
			return err
		}
	}
	return os.Rename(partial, directory)
}

// Range 请求只读取所需 ZIP 成员；断点文件保存压缩字节，解压后校验固定摘要。
func downloadRange(ctx context.Context, client *http.Client, source, partial string, start, length, total int64) (err error) {
	file, err := os.OpenFile(partial, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	offset := info.Size()
	if offset > length {
		return errors.New("partial vector asset exceeds expected size")
	}
	if offset == length {
		return nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start+offset, start+length-1))
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, response.Body.Close()) }()
	switch response.StatusCode {
	case http.StatusPartialContent:
		expected := fmt.Sprintf("bytes %d-%d/%d", start+offset, start+length-1, total)
		if response.Header.Get("Content-Range") != expected {
			return errors.New("invalid vector asset Content-Range")
		}
	case http.StatusOK:
		if start != 0 || length != total {
			return errors.New("native package server did not honor the byte range")
		}
		if err = file.Truncate(0); err != nil {
			return err
		}
		offset = 0
	default:
		return fmt.Errorf("vector asset download: HTTP %d", response.StatusCode)
	}
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	n, err := io.Copy(file, response.Body)
	if err != nil {
		return err
	}
	if n != length-offset {
		return errors.New("incomplete vector asset download")
	}
	return file.Sync()
}

func verifyFile(ctx context.Context, source, target string, a asset) (err error) {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	digest := sha256.New()
	size, copyErr := io.Copy(digest, assetReader{ctx, file})
	if err = errors.Join(copyErr, file.Close()); err != nil {
		return err
	}
	if size != a.size || hex.EncodeToString(digest.Sum(nil)) != a.digest {
		return errors.Join(errors.New("vector asset integrity check failed"), os.Remove(source))
	}
	return os.Rename(source, target)
}

func unpackFile(ctx context.Context, source, target string, a asset, method uint16) (err error) {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, input.Close()) }()
	var reader io.Reader = input
	if method == zip.Deflate {
		decoder := flate.NewReader(input)
		defer func() { err = errors.Join(err, decoder.Close()) }()
		reader = decoder
	} else if method != zip.Store {
		return errors.New("unsupported native package compression")
	}
	output, err := os.OpenFile(target+".unpacked", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	digest := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(output, digest), assetReader{ctx, reader})
	if err = errors.Join(copyErr, output.Sync(), output.Close()); err != nil {
		return err
	}
	if size != a.size || hex.EncodeToString(digest.Sum(nil)) != a.digest {
		return errors.Join(errors.New("native asset integrity check failed"), os.Remove(source))
	}
	if err = os.Rename(target+".unpacked", target); err != nil {
		return err
	}
	return nil
}

type assetReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r assetReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

type rangeArchive struct {
	ctx    context.Context
	client *http.Client
	url    string
	size   int64
}

func openArchive(ctx context.Context, client *http.Client, url string) (result *rangeArchive, err error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept-Encoding", "identity")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, response.Body.Close()) }()
	if response.StatusCode != http.StatusOK || response.ContentLength <= 0 {
		return nil, errors.New("could not determine native package size")
	}
	return &rangeArchive{ctx, client, url, response.ContentLength}, nil
}
func (r *rangeArchive) ReadAt(p []byte, offset int64) (n int, err error) {
	if offset < 0 {
		return 0, errors.New("negative native package offset")
	}
	if offset >= r.size {
		return 0, io.EOF
	}
	length := min(int64(len(p)), r.size-offset)
	request, err := http.NewRequestWithContext(r.ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
	response, err := r.client.Do(request)
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, response.Body.Close()) }()
	if response.StatusCode != http.StatusPartialContent || response.Header.Get("Content-Range") != fmt.Sprintf("bytes %d-%d/%d", offset, offset+length-1, r.size) {
		return 0, errors.New("invalid native package range response")
	}
	n, err = io.ReadFull(response.Body, p[:length])
	if err == nil && n < len(p) {
		err = io.EOF
	}
	return n, err
}

// ClearUnused 删除当前配置之外的已安装模型及运行库；调用方持有同步锁。
func ClearUnused(directory, name, device string) error {
	var keepModel, keepRuntime, keepGPU string
	if IsLocal(name) {
		_, _, modelDir, runtimeDir, err := localPaths(directory, name)
		if err != nil {
			return err
		}
		keepModel, keepRuntime = filepath.Base(modelDir), filepath.Base(runtimeDir)
		if device == "gpu" {
			_, gpu := gpuPath(directory)
			keepGPU = filepath.Base(gpu)
		}
	}
	for _, group := range []struct{ folder, keep string }{{"models", keepModel}, {"runtime", keepRuntime}} {
		parent := filepath.Join(directory, group.folder)
		entries, err := os.ReadDir(parent)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if (group.keep != "" && (entry.Name() == group.keep || entry.Name() == group.keep+".partial")) || (group.folder == "runtime" && keepGPU != "" && (entry.Name() == keepGPU || entry.Name() == keepGPU+".partial")) {
				continue
			}
			if err = os.RemoveAll(filepath.Join(parent, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}
