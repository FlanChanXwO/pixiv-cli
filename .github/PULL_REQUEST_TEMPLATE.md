<!-- Localized instructions: English / 中文 -->

<!--
提交前确认：不含 refresh token、cookie、authorization code、代理凭据、私有 URL、下载作品、本地数据库或私有 API 响应。
Before submitting: do not include refresh tokens, cookies, authorization codes, proxy credentials, private URLs, downloaded works, local databases, or private API responses.
-->

## Summary / 概述

<!--
English: Describe the user problem and the change that addresses it. Link related issues with "Closes #123" when applicable.
中文：说明用户问题及解决该问题的改动。适用时使用 “Closes #123” 关联 Issue。
-->

## Scope and compatibility / 范围与兼容性

<!--
English: List affected CLI commands or flags, MCP tools or schemas, public SDK APIs, configuration, environment variables, output contracts, and release behavior. State "None" when there is no public impact.
中文：列出受影响的 CLI command 或 flag、MCP tool 或 schema、public SDK API、配置、环境变量、输出契约和发布行为。没有公开影响时填写 “None”。
-->

## Verification / 验证

<!--
English: List the exact commands you ran and their results. For real Pixiv or FANBOX API coverage, state whether it was run and use only redacted evidence.
中文：列出实际运行的精确命令及结果。真实 Pixiv 或 FANBOX API 覆盖须说明是否运行，且只提供脱敏证据。
-->

```text
go test ./...
sh scripts/build.sh
```

## Checklist / 检查清单

- [ ] The change is focused and linked to an issue when appropriate. / 改动目标明确，并在适用时关联 Issue。
- [ ] I added or updated focused tests for changed behavior. / 我已为变更行为新增或更新聚焦测试。
- [ ] I ran the relevant tests and recorded the results above. / 我已运行相关测试并在上方记录结果。
- [ ] I updated the required CLI, MCP, SDK, README, maintainer, and product-skill documentation. / 我已更新所需的 CLI、MCP、SDK、README、维护者和产品 skill 文档。
- [ ] I documented every new timeout, retry, pagination or result limit, truncation, fallback, or downgrade and its evidence. / 我已记录每项新增 timeout、retry、pagination 或结果限制、truncation、fallback 或 downgrade 及其证据。
- [ ] I did not add refresh tokens, cookies, authorization codes, proxy credentials, private URLs, downloaded works, local state, or private API responses. / 我没有添加 refresh token、cookie、authorization code、代理凭据、私有 URL、下载作品、本地状态或私有 API 响应。
- [ ] I updated migration guidance for every breaking change. / 我已为每个破坏性变更更新迁移指引。
