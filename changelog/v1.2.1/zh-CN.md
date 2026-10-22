# v1.2.1 — 2026-10-22

## 修复

- 将 fsnotify 升级到 v1.10.1，修复 race detector 报告的 Windows 账号存储监听器并发访问问题。([#153](https://github.com/FlanChanXwO/pixiv-cli/pull/153))

## 安全

- 将源码构建、CI 和发布二进制所需的 Go 工具链从 1.27.1 提升至 1.27.3。Go 1.27.3 通过在 `crypto/x509` 中拒绝包含特定不可信中间证书的证书链，修复 CVE-2026-111943；本次升级也包含 Go 1.27.2 的安全修复。([#158](https://github.com/FlanChanXwO/pixiv-cli/pull/158)、[Go 安全公告](https://groups.google.com/g/golang-announce/c/66pojueaQek)、[Go 发布历史](https://go.dev/doc/devel/release#go1.27.2))

## 文档

- 以表格和图示重组双语文档，将安装、认证、MCP 图库及维护指南拆为聚焦的独立页面，并修正快速开始命令。([#154](https://github.com/FlanChanXwO/pixiv-cli/pull/154))
- 在双语 README 中加入动态贡献者展示和链接。([#156](https://github.com/FlanChanXwO/pixiv-cli/pull/156))

## 维护

- 将根目录 LICENSE 恢复为纯 MIT 文本，以便 GitHub 正确识别许可证；第三方声明仍通过独立许可包分发。([#155](https://github.com/FlanChanXwO/pixiv-cli/pull/155))
- 更新 PR 依赖检查清单，使其符合纯 Go 项目的现状。([#157](https://github.com/FlanChanXwO/pixiv-cli/pull/157))

**完整变更**：[v1.2.0...v1.2.1](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.2.0...v1.2.1)
