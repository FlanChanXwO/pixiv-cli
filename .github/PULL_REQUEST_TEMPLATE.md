<!--
提交前确认：不含 token、cookie、私有地址、下载内容或本地数据。
Before submitting: no tokens, cookies, private URLs, downloads, or local data.
-->

<!--
Closes #123
-->

## 变更点 / Changes

<!--
列要点：改动 + 原因。关联 Issue 用 `Closes #123`（合并后关闭）。
Bullet the change and why. Link an issue with `Closes #123` (closes on merge).
-->

## 验证步骤 / Verification

<!--
实际运行的命令和结果/截图。例如 / Commands and results/Screenshot. E.g.
- `go test ./...`
- `sh scripts/build.sh`
未测试时说明原因。 / If not tested, explain why.
-->

## 检查清单 / Checklist

- [ ] 我没有引入恶意代码 / No malicious code
- [ ] 我没有新增依赖，或已在 `go.mod`（或 Rust `Cargo.toml`）的补充新的依赖 / No new dependencies, or added name, source, and purpose to `go.mod` (or Rust `Cargo.toml`) in Changes
- [ ] 这不是一次破坏性更新，或已在「变更点」标注迁移影响 / Not a breaking change, or migration impact noted in Changes
- [ ] 受影响的文档已在 `docs/en/` 与 `docs/zh-CN/` 同步 / Affected docs synced under `docs/en/` and `docs/zh-CN/`
