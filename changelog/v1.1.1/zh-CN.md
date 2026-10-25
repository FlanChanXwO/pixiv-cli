# v1.1.1 — 2026-09-23

## 变更

- 发布现在只需要一次最终确认：构建、SDK 端到端检查、源码与产物校验、生产构建和容器检查会自动运行；确认后发布 GitHub Release 及下游包。（[#103](https://github.com/FlanChanXwO/pixiv-cli/pull/103), [#104](https://github.com/FlanChanXwO/pixiv-cli/pull/104), [#108](https://github.com/FlanChanXwO/pixiv-cli/pull/108), [#121](https://github.com/FlanChanXwO/pixiv-cli/pull/121)）
- ClawHub 发布会在 registry 中看到已发布的 skill 后再校验。（[#87](https://github.com/FlanChanXwO/pixiv-cli/pull/87)）

## 安全

- 自动化的 pull request 检查针对实际的 head 提交运行，检查名称保持稳定；fork 或修改 base 分支后不会复用旧结果。（[#100](https://github.com/FlanChanXwO/pixiv-cli/pull/100), [#101](https://github.com/FlanChanXwO/pixiv-cli/pull/101), [#102](https://github.com/FlanChanXwO/pixiv-cli/pull/102), [#107](https://github.com/FlanChanXwO/pixiv-cli/pull/107), [#111](https://github.com/FlanChanXwO/pixiv-cli/pull/111), [#115](https://github.com/FlanChanXwO/pixiv-cli/pull/115), [#117](https://github.com/FlanChanXwO/pixiv-cli/pull/117), [#119](https://github.com/FlanChanXwO/pixiv-cli/pull/119), [#120](https://github.com/FlanChanXwO/pixiv-cli/pull/120), [#88](https://github.com/FlanChanXwO/pixiv-cli/pull/88), [#89](https://github.com/FlanChanXwO/pixiv-cli/pull/89), [#90](https://github.com/FlanChanXwO/pixiv-cli/pull/90), [#91](https://github.com/FlanChanXwO/pixiv-cli/pull/91), [#93](https://github.com/FlanChanXwO/pixiv-cli/pull/93), [#94](https://github.com/FlanChanXwO/pixiv-cli/pull/94), [#99](https://github.com/FlanChanXwO/pixiv-cli/pull/99)）

**完整变更**：[v1.1.0...v1.1.1](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.1.1)

<!-- internal:
  [#105](https://github.com/FlanChanXwO/pixiv-cli/pull/105)
  [#109](https://github.com/FlanChanXwO/pixiv-cli/pull/109)
  [#110](https://github.com/FlanChanXwO/pixiv-cli/pull/110)
  [#114](https://github.com/FlanChanXwO/pixiv-cli/pull/114)
  [#92](https://github.com/FlanChanXwO/pixiv-cli/pull/92)
  [#96](https://github.com/FlanChanXwO/pixiv-cli/pull/96)
  [#97](https://github.com/FlanChanXwO/pixiv-cli/pull/97)
  [#98](https://github.com/FlanChanXwO/pixiv-cli/pull/98)
-->
