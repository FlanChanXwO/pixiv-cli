//go:build !windows

package atomicfile

func replacePrivateFile(sourcePath, targetPath string) (Result, error) {
	err := ReplaceFile(sourcePath, targetPath)
	return Result{Committed: err == nil}, err
}
