// Package selfupdate 编排显式更新与自动检查。
//
// 包的组成：[UpdateCoordinator] 执行一次更新请求，[AutomaticUpdateChecker] 周期检查
// 并在 [AutomaticUpdateNotice] 中返回提示，[CommandRunner] 运行不经 shell 的更新命令；
// 来源选择、release 策略与安装保留在各自的子包中。
package selfupdate
