# v1.0.1 — 2026-09-02

## 新增

- 新增浏览器兼容的 ascii2d transport，并通过 FlareSolverr JSON control API 提供可选的 challenge recovery；standard/source 与 solver-browser proxy 相互独立。native ascii2d upload 仍直接使用 multipart，solver 不会收到图片上传。([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))
- 在连续 MCP 调用间复用有效的 FlareSolverr state，合并并发 solve，并随 CLI 或 MCP 生命周期关闭 provider 与 solver client。([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))

## 变更

- 为 ascii2d 匹配 Chrome browser User-Agent 与 client hints，拒绝不一致的自定义身份值，并将 ascii2d proxy 与 standard source、SauceNAO 流量分离。([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))
- 明确识别 challenge response，使用 solver 提供的 session 对 native request 最多重放一次，并一致解析 live ascii2d result page。([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))

## 修复

- 将 FlareSolverr 不可用、失败或格式错误映射为稳定的 public reverse-search error code，不暴露 wrapped cause 或 upstream body。([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))
- 让 reverse-search E2E 与 MCP runner cleanup 等待进程退出并关闭 client，避免验证和长生命周期运行中泄漏资源。([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))

## 安全

- 保持 challenge recovery 仅用于 challenge 且仅使用 JSON control：solver 流量不携带 native multipart image data，solver state 不写入磁盘，source、credential、cookie、CSRF、临时路径和 upstream body 不进入公开输出或诊断。([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))

## 文档

- 补充 reverse-search transport 与 proxy 分离、Chrome-146 identity pairing、仅限 challenge 的 solver 边界、provider upload limit、稳定 solver error 以及 CLI/MCP lifecycle contract 文档。([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))

## 维护

- 补充 ascii2d transport 与 challenge classification、solver lifecycle 与复用、live result parsing、all-provider aggregation 和 MCP cleanup 的聚焦测试。([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))

**完整变更**：[v1.0.0...v1.0.1](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.0...v1.0.1)
