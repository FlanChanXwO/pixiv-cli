# ADR 0008: Ed25519 签名的多渠道发布信任

## Status

Accepted；production rollout 尚未开始。

## Context

`pixiv update` 需要根据本机安装来源选择 Homebrew、`go install` 或 Release binary 更新。不同渠道的
更新器不能把“GitHub 返回一个 asset URL”视为足够信任：Release binary 需要验证 archive 的内容和
版本，Homebrew stable/beta 需要避免两个都提供 `pixiv`，而 Go 安装需要精确绑定已选 tag。用户也必须
了解 v0.1.0 没有 Apple notarization/Windows Authenticode，直接下载可能触发系统信誉提示。

发布签名私钥、tap 写权限和 GitHub Actions token 具有不同权限范围。把它们提交到源码、放进普通
repository secret，或复用同一 SSH key，会让任意 tag/workflow 或公开历史取得不必要的发布能力。

## Decision

- GitHub Releases API 是更新查询的唯一后端。draft 不可选；自动检查只选 stable，显式
  `--prerelease` 才可选择预发布版本。
- 每个正式 Release 使用固定六目标 asset 名，发布 `checksums.txt` 以及对其原始 bytes 签名、含 key
  ID 的 Ed25519 `checksums.json`。Release installer 先验证签名和 SHA-256，再解包、版本预检并原子
  替换。
- Release binary 只信任随受支持 binary 提交的 Ed25519 public key/key ID。私钥只允许存在于受保护
  `release` Environment secret；恢复副本只可存在于受控 macOS Keychain。私钥不能进入 CLI、源码、
  GitHub Release、formula、日志或测试 fixture。
- key rotation 先发布同时信任新 key ID 的 binary，保留旧 key 直到旧 binary 退出支持，再以新的
  受签名 Release 停用旧 key。不得要求已发布 binary 突然信任一个未随其发布的 key。
- Homebrew 使用独立 tap deploy key：tap repository 只登记公钥，source repository 的受保护
  `release` Environment 才保存私钥。stable `pixiv-cli` 与 beta `pixiv-cli-beta` 相互冲突，并由
  已验证 Release checksum 渲染；Go 通过精确 tag 更新。
- 发布 workflow 必须先完成六 target build/test/license/archive 门禁，且在受保护 Environment 中签名；
  草稿 Release 的 asset 集合经核验后才可公开。Homebrew audit/安装通过后才可 push tap。

## Consequences

- 自动检查是只读、限时且 stderr-only 的提示；它不能修改业务退出码或 JSON/MCP stdout。显式更新必须
  将渠道切换、权限、HTTP、签名、checksum、archive 和替换失败如实暴露。
- 当前没有 production public key/key ID、私钥、Keychain backup、受保护 Environment、公开 remote、
  GitHub Release 或 tap。Release installer 因此明确报告缺少 trust root；不能将 `--check` 成功、
  本地 fixture 或 workflow 文件视为已完成的安全发布。
- 当前 staticlib/manifest、workflow policy 和 native artifact evidence 仍未齐备，必须在创建 tag、写入
  secret、发布 Release 或推送 tap 前完成。
- v0.1.0 用户仍可能看到 Gatekeeper/SmartScreen 警告；支持文档应指导用户回到已验证 Release、
  checksum 和签名记录，而不是绕过系统信誉机制。
