//go:build !windows

package download

import "os"

func replaceDownloadedFile(tmpPath, targetPath string) error {
	return os.Rename(tmpPath, targetPath)
}
