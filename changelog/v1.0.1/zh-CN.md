# v1.0.1 — 2026-09-02

## 变更

- ascii2d 反向搜图改用浏览器兼容的传输，匹配 Chrome User-Agent 与 client hints，其代理设置与 SauceNAO、普通来源流量分开。可选 FlareSolverr JSON 控制接口用于处理挑战；solver 不会收到上传的图片。（[#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75)）

## 修复

- FlareSolverr 不可用、失败或响应格式错误时会返回稳定的错误码，不再泄露上游响应内容；solver 状态会在多次 MCP 调用间复用，并随 CLI 或 MCP 生命周期释放。（[#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75)）

## 安全

- 挑战处理仅限挑战响应和 JSON 控制：不向 solver 发送图片数据，solver 状态不写入磁盘，来源、凭据、cookie、CSRF、临时路径和上游响应内容不会进入输出或诊断。（[#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75)）

**完整变更**：[v1.0.0...v1.0.1](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.0...v1.0.1)
