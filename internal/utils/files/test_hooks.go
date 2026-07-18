package files

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
