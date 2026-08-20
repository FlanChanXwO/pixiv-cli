//go:build !windows

package replace

func replacePrivateFile(sourcePath, targetPath string) (Result, error) {
	err := ReplaceFile(sourcePath, targetPath)
	return Result{Committed: err == nil}, err
}
