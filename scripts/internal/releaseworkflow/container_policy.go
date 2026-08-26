package releaseworkflow

import (
	"errors"

	"gopkg.in/yaml.v3"
)

// containerRegistry 是 GHCR 上 pixiv-cli 容器镜像的仓库路径。
const containerRegistry = "ghcr.io/flanchanxwo/pixiv-cli"

// containerMatrixTargets 绑定两个原生 Linux 容器构建 target 与其 release matrix identity，
// 复用 build_production 的 Linux target 定义，确保 ABI provenance 一致。
var containerMatrixTargets = map[string]struct{}{
	"ubuntu-22.04|linux|amd64|x86_64-unknown-linux-gnu|linux-amd64|gcc":      {},
	"ubuntu-22.04-arm|linux|arm64|aarch64-unknown-linux-gnu|linux-arm64|gcc": {},
}

// checkContainerBuildJob 校验 build_container job 的容器构建 contract。
// Task 2 将实现完整规则：原生 Linux runner、不可变 tag checkout、无 packages: write、无 QEMU。
func checkContainerBuildJob(job *yaml.Node) error {
	return errors.New("container build job contract not yet implemented")
}

// checkContainerPublishJob 校验 publish_container job 的 GHCR 发布 contract。
// Task 2 将实现完整规则：唯一 packages: write、依赖 build_container 和 publish、stable/prerelease tag 语义。
func checkContainerPublishJob(job *yaml.Node) error {
	return errors.New("container publish job contract not yet implemented")
}
