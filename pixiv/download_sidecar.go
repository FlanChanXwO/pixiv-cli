package pixiv

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
)

// downloadSidecar 是每个用户可见产物的公开元数据。Illust 保留完整 public SDK
// 模型；artifact 字段只描述本地输出，不包含请求 URL、凭据或其他会话信息。
type downloadSidecar struct {
	SchemaVersion int                  `json:"schema_version"`
	Artifact      string               `json:"artifact"`
	Page          int                  `json:"page,omitempty"`
	UgoiraMode    UgoiraMode           `json:"ugoira_mode,omitempty"`
	Frames        []ugoiraFrameSidecar `json:"frames,omitempty"`
	Illust        Illust               `json:"illust"`
}

type ugoiraFrameSidecar struct {
	File  string `json:"file"`
	Delay int    `json:"delay"`
}

func writeDownloadSidecars(base string, result DownloadResult, illust Illust, mode UgoiraMode, frames []ugoiraFrameSidecar) error {
	for _, file := range result.Files {
		relative, err := filepath.Rel(base, file.Path)
		if err != nil || filepath.IsAbs(relative) || relative == ".." {
			return invalidResourceError(OperationDownload, "cannot create metadata sidecar path")
		}
		body, err := json.Marshal(downloadSidecar{
			SchemaVersion: 1, Artifact: filepath.ToSlash(relative), Page: file.Page,
			UgoiraMode: mode, Frames: frames, Illust: illust,
		})
		if err != nil {
			return invalidResourceError(OperationDownload, "cannot encode metadata sidecar")
		}
		if err := writeAtomicSidecar(file.Path+".json", body); err != nil {
			return err
		}
	}
	return nil
}

func writeAtomicSidecar(destination string, body []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".pixiv-metadata-*")
	if err != nil {
		return invalidResourceError(OperationDownload, "cannot create metadata sidecar")
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
		return invalidResourceError(OperationDownload, "cannot protect metadata sidecar")
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return invalidResourceError(OperationDownload, "cannot write metadata sidecar")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return invalidResourceError(OperationDownload, "cannot sync metadata sidecar")
	}
	if err := temporary.Close(); err != nil {
		return invalidResourceError(OperationDownload, "cannot close metadata sidecar")
	}
	if err := files.ReplaceFile(temporaryPath, destination); err != nil {
		if files.MustPreserveReplacementSource(err) {
			keepTemporary = false
		}
		return fmt.Errorf("write metadata sidecar: %w", err)
	}
	keepTemporary = false
	return nil
}
