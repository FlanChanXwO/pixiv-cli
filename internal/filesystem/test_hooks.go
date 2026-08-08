package filesystem

import "os"

// WritePrivateFileWithSyncParentForTest 使用真实 private writer，仅替换目录同步边界。
// 它用于稳定复现 replacement 已提交后的 durability failure；生产代码不得调用。
func WritePrivateFileWithSyncParentForTest(path string, body []byte, mode os.FileMode, syncParent func(string) error) error {
	if syncParent == nil {
		panic("private file sync-parent test hook requires a function")
	}
	ops := defaultPrivateFileOps()
	ops.syncParent = syncParent
	return writePrivateFile(path, body, mode, ops)
}

// WritePrivateFileWithUnresolvedReplacementForTest 使用真实 staging，并模拟旧目标已
// 移入 recovery backup、目标恢复未决的 replacement outcome。新旧 recovery
// artifacts 都必须保留，生产代码不得调用。
func WritePrivateFileWithUnresolvedReplacementForTest(path string, body []byte, mode os.FileMode, cause error) error {
	if cause == nil {
		panic("private file unresolved-replacement test hook requires an error")
	}
	ops := defaultPrivateFileOps()
	ops.replaceFile = func(sourcePath, targetPath string) (privateFileReplaceOutcome, error) {
		if err := os.Rename(targetPath, sourcePath+".recovery"); err != nil {
			return privateFileReplaceOutcome{}, err
		}
		return privateFileReplaceOutcome{preserveSource: true}, cause
	}
	return writePrivateFile(path, body, mode, ops)
}
