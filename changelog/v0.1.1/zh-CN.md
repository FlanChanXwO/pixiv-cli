# v0.1.1 — 2026-07-13

## 修复

- 修复直接下载的 Release binary 在本机不存在预期 `GOBIN`/`GOPATH/bin/pixiv` 时，把该正常安装来源判定为错误而无法执行 `pixiv update --check` 的问题；不存在的 go install 目标现在会正确归类为 Release，其他路径解析错误仍会原样报告。
- 修复 Release workflow 在 Windows 上以 MinGW GCC 链接 MSVC Rust staticlib 的错误；六平台 Go 测试、race、vet、pre-commit 与最终构建现在统一使用各自受审计的 cgo linker，Windows 固定为 LLD-backed Clang。
- 修复登录测试夹具对回调 URL 列表的并发读写，并隔离不应访问真实 macOS URL handler/AppleScript 的场景，避免 race detector 报错或冷 runner 因系统 helper 副作用耗尽显式测试等待窗口。
- 新增不可变 tag 首次发布在创建 Release 前失败时的受审计恢复入口；恢复仍绑定原 tag，测试门禁与生产资产使用独立 runner，后者以 clean checkout 重建工作树和 staticlib，禁止默认分支测试 overlay 或其进程环境混入 binary、许可证或归档。
- 修复恢复测试门在 Windows runner 上对 ACL、`.exe`、CRLF、文件共享和路径转义的错误假设；覆盖路径受静态 policy 限制，生产资产仍只由不可变 tag 源码构建。
