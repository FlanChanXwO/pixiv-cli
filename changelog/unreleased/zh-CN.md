# 未发布

> Release-prep 暂存区：审计目标 tag 范围后直接写入下一版双语说明。审计范围内的每个 PR 或 direct commit 都需出现在两种语言中；仅内部改动列在末尾的 `internal` 注释里，渲染 Release 正文时不会显示。

## 破坏性变更

- `pixiv mcp` 不再接受 `mcp_gallery_cache_ttl` 与 `mcp_gallery_concurrency` 设置，请从 `config.toml` 中删除。（[#168](https://github.com/FlanChanXwO/pixiv-cli/pull/168)）

## 新增

- MCP 新增 `pixiv_show_artwork` 工具，Gallery 布局会适配手机屏幕和嵌入式面板。（[#167](https://github.com/FlanChanXwO/pixiv-cli/pull/167)）

## 变更

- 收藏读取部分失败时，MCP 搜索筛选条件会保留，并报告部分结果，不再丢弃筛选条件或已成功读取的条目。（[#165](https://github.com/FlanChanXwO/pixiv-cli/pull/165)）
- 账号列表为空时编码为 JSON `null` 而不是 `[]`，保持现有 CLI JSON 契约。（[#201](https://github.com/FlanChanXwO/pixiv-cli/pull/201)）


**完整变更**：打下一版 tag 时补充。

<!-- internal:
  [#230](https://github.com/FlanChanXwO/pixiv-cli/pull/230)
  [#231](https://github.com/FlanChanXwO/pixiv-cli/pull/231)
  [#228](https://github.com/FlanChanXwO/pixiv-cli/pull/228)
  [#229](https://github.com/FlanChanXwO/pixiv-cli/pull/229)
  [#227](https://github.com/FlanChanXwO/pixiv-cli/pull/227)
  [#218](https://github.com/FlanChanXwO/pixiv-cli/pull/218)
  [#220](https://github.com/FlanChanXwO/pixiv-cli/pull/220)
  [#222](https://github.com/FlanChanXwO/pixiv-cli/pull/222)
  [#223](https://github.com/FlanChanXwO/pixiv-cli/pull/223)
  [#224](https://github.com/FlanChanXwO/pixiv-cli/pull/224)
  [#225](https://github.com/FlanChanXwO/pixiv-cli/pull/225)
  [#226](https://github.com/FlanChanXwO/pixiv-cli/pull/226)
  [#177](https://github.com/FlanChanXwO/pixiv-cli/pull/177)
  [#183](https://github.com/FlanChanXwO/pixiv-cli/pull/183)
  [#185](https://github.com/FlanChanXwO/pixiv-cli/pull/185)
  [#159](https://github.com/FlanChanXwO/pixiv-cli/pull/159)
  [#160](https://github.com/FlanChanXwO/pixiv-cli/pull/160)
  [#161](https://github.com/FlanChanXwO/pixiv-cli/pull/161)
  [#162](https://github.com/FlanChanXwO/pixiv-cli/pull/162)
  [#163](https://github.com/FlanChanXwO/pixiv-cli/pull/163)
  [#164](https://github.com/FlanChanXwO/pixiv-cli/pull/164)
  [#166](https://github.com/FlanChanXwO/pixiv-cli/pull/166)
  [#170](https://github.com/FlanChanXwO/pixiv-cli/pull/170)
  [#171](https://github.com/FlanChanXwO/pixiv-cli/pull/171)
  [#172](https://github.com/FlanChanXwO/pixiv-cli/pull/172)
  [#173](https://github.com/FlanChanXwO/pixiv-cli/pull/173)
  [#174](https://github.com/FlanChanXwO/pixiv-cli/pull/174)
  [#175](https://github.com/FlanChanXwO/pixiv-cli/pull/175)
  [#176](https://github.com/FlanChanXwO/pixiv-cli/pull/176)
  [#178](https://github.com/FlanChanXwO/pixiv-cli/pull/178)
  [#179](https://github.com/FlanChanXwO/pixiv-cli/pull/179)
  [#180](https://github.com/FlanChanXwO/pixiv-cli/pull/180)
  [#181](https://github.com/FlanChanXwO/pixiv-cli/pull/181)
  [#182](https://github.com/FlanChanXwO/pixiv-cli/pull/182)
  [#184](https://github.com/FlanChanXwO/pixiv-cli/pull/184)
  [#186](https://github.com/FlanChanXwO/pixiv-cli/pull/186)
  [#187](https://github.com/FlanChanXwO/pixiv-cli/pull/187)
  [#188](https://github.com/FlanChanXwO/pixiv-cli/pull/188)
  [#189](https://github.com/FlanChanXwO/pixiv-cli/pull/189)
  [#190](https://github.com/FlanChanXwO/pixiv-cli/pull/190)
  [#191](https://github.com/FlanChanXwO/pixiv-cli/pull/191)
  [#192](https://github.com/FlanChanXwO/pixiv-cli/pull/192)
  [#193](https://github.com/FlanChanXwO/pixiv-cli/pull/193)
  [#194](https://github.com/FlanChanXwO/pixiv-cli/pull/194)
  [#195](https://github.com/FlanChanXwO/pixiv-cli/pull/195)
  [#196](https://github.com/FlanChanXwO/pixiv-cli/pull/196)
  [#197](https://github.com/FlanChanXwO/pixiv-cli/pull/197)
  [#198](https://github.com/FlanChanXwO/pixiv-cli/pull/198)
  [#199](https://github.com/FlanChanXwO/pixiv-cli/pull/199)
  [#200](https://github.com/FlanChanXwO/pixiv-cli/pull/200)
  [#202](https://github.com/FlanChanXwO/pixiv-cli/pull/202)
  [#203](https://github.com/FlanChanXwO/pixiv-cli/pull/203)
  [#204](https://github.com/FlanChanXwO/pixiv-cli/pull/204)
  [#205](https://github.com/FlanChanXwO/pixiv-cli/pull/205)
  [#206](https://github.com/FlanChanXwO/pixiv-cli/pull/206)
  [#207](https://github.com/FlanChanXwO/pixiv-cli/pull/207)
  [#208](https://github.com/FlanChanXwO/pixiv-cli/pull/208)
  [#209](https://github.com/FlanChanXwO/pixiv-cli/pull/209)
  [#210](https://github.com/FlanChanXwO/pixiv-cli/pull/210)
  [#211](https://github.com/FlanChanXwO/pixiv-cli/pull/211)
  [#212](https://github.com/FlanChanXwO/pixiv-cli/pull/212)
  [#213](https://github.com/FlanChanXwO/pixiv-cli/pull/213)
  [#214](https://github.com/FlanChanXwO/pixiv-cli/pull/214)
  [#215](https://github.com/FlanChanXwO/pixiv-cli/pull/215)
  [#216](https://github.com/FlanChanXwO/pixiv-cli/pull/216)
  [#217](https://github.com/FlanChanXwO/pixiv-cli/pull/217)
  [#219](https://github.com/FlanChanXwO/pixiv-cli/pull/219)
  [#221](https://github.com/FlanChanXwO/pixiv-cli/pull/221)
-->
