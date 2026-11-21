# v1.5.0 — 2026-11-21

## 破坏性变更

- 向量索引自动从 schema v1/v2 升级至 v3，保留已有记录、向量和扫描断点。旧版程序会拒绝读取升级后的索引；如需回退旧版本，请在升级前备份向量索引。 ([#305](https://github.com/FlanChanXwO/pixiv-cli/pull/305))

## 新增

- `pixiv vector sync --limit 100` 可同步前 100 个收藏，`--offset 500 --limit 10` 同步第 501–510 个，`--tail 10` 同步最后 10 个。公开和私密列表分别计数，先选位置再应用类型／分级筛选。精确选取尾部需要完整遍历所选列表。 ([#305](https://github.com/FlanChanXwO/pixiv-cli/pull/305))

- 范围同步中断后重跑相同命令即可续跑，复用已完成的可见性范围和有效向量，并保留普通同步断点及选区外的待算任务。终端显示范围进度，sync JSON 新增 `range`，`vector status --json` 通过 `ranges` 展示未完成任务。 ([#305](https://github.com/FlanChanXwO/pixiv-cli/pull/305))

**完整变更：** [v1.4.0...v1.5.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.4.0...v1.5.0)
