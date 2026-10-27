// Package installer 安装已校验的 release 二进制，并持有签名信任根。
//
// 包的组成：[ReleaseInstaller] 与 [ReleaseInstallerOptions] 负责下载、校验和替换
// 可执行文件，[SourceDetector] 与 [InstallSource] 识别安装来源，
// [NewFileReleaseCache] 提供私有缓存；缓存权限与待处理更新清理也由本包负责。
package installer
