# v1.0.1 — 2026-09-02

## 新增

- 新增浏览器兼容的 ascii2d transport，并通过 FlareSolverr JSON control API 提供可选的 challenge recovery；standard/source 与 solver-browser proxy 相互独立。native ascii2d upload 仍直接使用 multipart，solver 不会收到图片上传。([`6071506`](https://github.com/FlanChanXwO/pixiv-cli/commit/60715066c28a2e5378f730e9af73205303b1728a), [`50d13a3`](https://github.com/FlanChanXwO/pixiv-cli/commit/50d13a3bf5b8f082816a0cf080f97039f4914a60), [`f9c1525`](https://github.com/FlanChanXwO/pixiv-cli/commit/f9c15256042dd69bec6a404cf69f4bb1ed0a629f))
- 在连续 MCP 调用间复用有效的 FlareSolverr state，合并并发 solve，并随 CLI 或 MCP 生命周期关闭 provider 与 solver client。([`f9c1525`](https://github.com/FlanChanXwO/pixiv-cli/commit/f9c15256042dd69bec6a404cf69f4bb1ed0a629f), [`341cfbb`](https://github.com/FlanChanXwO/pixiv-cli/commit/341cfbb233baabac57e9261f16e29946b2b043fc), [`e9ca76c`](https://github.com/FlanChanXwO/pixiv-cli/commit/e9ca76ce3bb2c9d9f1395183be01c4bddd14a463), [`05df0d5`](https://github.com/FlanChanXwO/pixiv-cli/commit/05df0d5a05c9fe6978eaddb7efb9e7d2bd8a0e00))

## 变更

- 为 ascii2d 匹配 Chrome browser User-Agent 与 client hints，拒绝不一致的自定义身份值，并将 ascii2d proxy 与 standard source、SauceNAO 流量分离。([`6071506`](https://github.com/FlanChanXwO/pixiv-cli/commit/60715066c28a2e5378f730e9af73205303b1728a), [`c01402a`](https://github.com/FlanChanXwO/pixiv-cli/commit/c01402afc69f0d510a78e911bc38f9ab0039532b), [`dde3aca`](https://github.com/FlanChanXwO/pixiv-cli/commit/dde3aca98f5850802d87d97cefb1705a1b0bc50c))
- 明确识别 challenge response，使用 solver 提供的 session 对 native request 最多重放一次，并一致解析 live ascii2d result page。([`7951909`](https://github.com/FlanChanXwO/pixiv-cli/commit/7951909d7491c82c59c283262b468650cd1c5784), [`50d13a3`](https://github.com/FlanChanXwO/pixiv-cli/commit/50d13a3bf5b8f082816a0cf080f97039f4914a60), [`341cfbb`](https://github.com/FlanChanXwO/pixiv-cli/commit/341cfbb233baabac57e9261f16e29946b2b043fc), [`471af9b`](https://github.com/FlanChanXwO/pixiv-cli/commit/471af9b9752c29bbe2f03f22c3cc6f4ae62c65f9), [`8337c6f`](https://github.com/FlanChanXwO/pixiv-cli/commit/8337c6f942db9522417df6d5d10e1aca225c1bf3))

## 修复

- 将 FlareSolverr 不可用、失败或格式错误映射为稳定的 public reverse-search error code，不暴露 wrapped cause 或 upstream body。([`471af9b`](https://github.com/FlanChanXwO/pixiv-cli/commit/471af9b9752c29bbe2f03f22c3cc6f4ae62c65f9))
- 让 reverse-search E2E 与 MCP runner cleanup 等待进程退出并关闭 client，避免验证和长生命周期运行中泄漏资源。([`05df0d5`](https://github.com/FlanChanXwO/pixiv-cli/commit/05df0d5a05c9fe6978eaddb7efb9e7d2bd8a0e00), [`a70c490`](https://github.com/FlanChanXwO/pixiv-cli/commit/a70c490e7000d4e4d123f0237a68539dadac550f))

## 安全

- 保持 challenge recovery 仅用于 challenge 且仅使用 JSON control：solver 流量不携带 native multipart image data，solver state 不写入磁盘，source、credential、cookie、CSRF、临时路径和 upstream body 不进入公开输出或诊断。([`50d13a3`](https://github.com/FlanChanXwO/pixiv-cli/commit/50d13a3bf5b8f082816a0cf080f97039f4914a60), [`f9c1525`](https://github.com/FlanChanXwO/pixiv-cli/commit/f9c15256042dd69bec6a404cf69f4bb1ed0a629f), [`341cfbb`](https://github.com/FlanChanXwO/pixiv-cli/commit/341cfbb233baabac57e9261f16e29946b2b043fc), [`e9ca76c`](https://github.com/FlanChanXwO/pixiv-cli/commit/e9ca76ce3bb2c9d9f1395183be01c4bddd14a463))

## 文档

- 补充 reverse-search transport 与 proxy 分离、Chrome-146 identity pairing、仅限 challenge 的 solver 边界、provider upload limit、稳定 solver error 以及 CLI/MCP lifecycle contract 文档。([`dde3aca`](https://github.com/FlanChanXwO/pixiv-cli/commit/dde3aca98f5850802d87d97cefb1705a1b0bc50c))

## 维护

- 补充 ascii2d transport 与 challenge classification、solver lifecycle 与复用、live result parsing、all-provider aggregation 和 MCP cleanup 的聚焦测试。([`6071506`](https://github.com/FlanChanXwO/pixiv-cli/commit/60715066c28a2e5378f730e9af73205303b1728a), [`c01402a`](https://github.com/FlanChanXwO/pixiv-cli/commit/c01402afc69f0d510a78e911bc38f9ab0039532b), [`7951909`](https://github.com/FlanChanXwO/pixiv-cli/commit/7951909d7491c82c59c283262b468650cd1c5784), [`50d13a3`](https://github.com/FlanChanXwO/pixiv-cli/commit/50d13a3bf5b8f082816a0cf080f97039f4914a60), [`f9c1525`](https://github.com/FlanChanXwO/pixiv-cli/commit/f9c15256042dd69bec6a404cf69f4bb1ed0a629f), [`341cfbb`](https://github.com/FlanChanXwO/pixiv-cli/commit/341cfbb233baabac57e9261f16e29946b2b043fc), [`471af9b`](https://github.com/FlanChanXwO/pixiv-cli/commit/471af9b9752c29bbe2f03f22c3cc6f4ae62c65f9), [`8337c6f`](https://github.com/FlanChanXwO/pixiv-cli/commit/8337c6f942db9522417df6d5d10e1aca225c1bf3), [`05df0d5`](https://github.com/FlanChanXwO/pixiv-cli/commit/05df0d5a05c9fe6978eaddb7efb9e7d2bd8a0e00), [`a70c490`](https://github.com/FlanChanXwO/pixiv-cli/commit/a70c490e7000d4e4d123f0237a68539dadac550f))

**完整变更**：[v1.0.0...v1.0.1](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.0...v1.0.1)
