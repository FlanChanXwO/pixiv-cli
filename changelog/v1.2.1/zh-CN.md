# v1.2.1 — 2026-10-22

## 修复

- 将 fsnotify 升级到 v1.10.1，修复 race detector 报告的 Windows 账号存储监听器并发访问问题。([#153](https://github.com/FlanChanXwO/pixiv-cli/pull/153))

## 安全

- 将源码构建、CI 和发布二进制所需的 Go 工具链从 1.27.1 提升至 1.27.3。Go 1.27.3 通过在 `crypto/x509` 中拒绝包含特定不可信中间证书的证书链，修复 CVE-2026-111943；本次升级也包含 Go 1.27.2 的安全修复。([#158](https://github.com/FlanChanXwO/pixiv-cli/pull/158)、[Go 安全公告](https://groups.google.com/g/golang-announce/c/66pojueaQek)、[Go 发布历史](https://go.dev/doc/devel/release#go1.27.2))

**完整变更**：[v1.2.0...v1.2.1](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.2.0...v1.2.1)
