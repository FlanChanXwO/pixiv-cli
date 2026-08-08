//go:build !windows

package filesystem

func replacePrivateFile(sourcePath, targetPath string) (privateFileReplaceOutcome, error) {
	err := ReplaceFile(sourcePath, targetPath)
	return privateFileReplaceOutcome{committed: err == nil}, err
}
