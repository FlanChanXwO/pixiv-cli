package settings

// Store 是配置 schema、文件 precedence 与稀疏写回的唯一 authority。
// Files 只提供协议无关的路径/文件机制；Store 不缓存读取结果。
type Store struct {
	Files FileStore
}

// Current 每次从当前文件与环境生成新的 Snapshot。调用方应在一次 command/tool
// execution 开始时获取一次，并把该不可变快照传给后续依赖，避免运行中途配置撕裂。
func (s Store) Current() (Snapshot, error) {
	files, err := s.fileStore()
	if err != nil {
		return Snapshot{}, err
	}
	path, err := files.Path()
	if err != nil {
		return Snapshot{}, err
	}
	return LoadSnapshotAtWithFileStore(path, files)
}

type ConfigMutationResult struct {
	Alias       string
	EnvOverride string
	HasOverride bool
}

func (s Store) Path() (string, error) {
	files, err := s.fileStore()
	if err != nil {
		return "", err
	}
	return files.Path()
}
