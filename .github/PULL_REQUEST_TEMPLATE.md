<!-- Localized instructions: English / 中文 / 日本語 -->

## Summary

<!--
English: Describe the user problem and the change that addresses it. Link related issues with "Fixes #123" when applicable.
中文：说明用户问题及解决该问题的改动。适用时使用 “Fixes #123” 关联 Issue。
日本語：user problem と、それを解決する変更を説明してください。該当する場合は "Fixes #123" で関連 issue をリンクします。
-->

## Scope and compatibility

<!--
English: List affected CLI commands or flags, MCP tools or schemas, SDK APIs, configuration, environment variables, output contracts, and release behavior. State "None" when there is no public impact.
中文：列出受影响的 CLI command 或 flag、MCP tool 或 schema、SDK API、配置、环境变量、输出契约和发布行为。没有公开影响时填写 “None”。
日本語：影響を受ける CLI command または flag、MCP tool または schema、SDK API、configuration、environment variable、output contract、release behavior を列挙してください。public impact がない場合は "None" と記載します。
-->

## Verification

<!--
English: List the exact commands you ran and their results. For real Pixiv API coverage, state whether it was run and use only redacted evidence.
中文：列出实际运行的精确命令及结果。真实 Pixiv API 覆盖须说明是否运行，且只提供脱敏证据。
日本語：実行した正確な command と結果を列挙してください。real Pixiv API coverage は実行の有無を示し、マスク済みの証拠だけを使用してください。
-->

```text
go test ./...
```

## Release note declaration

<!--
English: This required metadata is validated by the Quality gate. It is used only after merge to prepare the bilingual release notes; do not edit changelog/unreleased in a feature PR. Select exactly one category: Added, Changed, Fixed, Security, Documentation, Maintenance, or None. Set breaking to true only when the change requires a major-version release. None requires a concrete reason.
中文：Quality gate 会校验此必填 metadata。它仅在合并后用于准备双语 release note；feature PR 中不要编辑 changelog/unreleased。只能选择一个类别：Added、Changed、Fixed、Security、Documentation、Maintenance 或 None。只有变更需要 major-version release 时才将 breaking 设为 true；None 必须提供具体理由。
日本語：この必須 metadata は Quality gate で検証されます。merge 後に bilingual release note を準備するためだけに使用し、feature PR で changelog/unreleased を編集しないでください。Added、Changed、Fixed、Security、Documentation、Maintenance、None から正確に 1 つ選びます。major-version release が必要な変更だけで breaking を true にし、None には具体的な理由が必要です。
-->

<!-- release-note
category:
breaking:
summary:
none_reason:
-->

## Checklist

- [ ] The change is focused and linked to an issue when appropriate. / 改动目标明确，并在适用时关联 Issue。 / 変更は焦点が絞られ、該当する場合は issue にリンクされています。
- [ ] I added or updated focused tests for changed behavior. / 我已为变更行为新增或更新聚焦测试。 / 変更された動作に対する focused test を追加または更新しました。
- [ ] I ran the relevant tests and recorded the results above. / 我已运行相关测试并在上方记录结果。 / 関連する test を実行し、結果を上に記録しました。
- [ ] I updated the required CLI, MCP, SDK, README, maintainer, and product-skill documentation. / 我已更新所需的 CLI、MCP、SDK、README、维护者和产品 skill 文档。 / 必要な CLI、MCP、SDK、README、maintainer、product-skill documentation を更新しました。
- [ ] I completed the required release-note declaration above; `None` has a concrete reason when selected. / 我已填写上方必需的 release-note 声明；选择 `None` 时已提供具体理由。 / 必須の release-note declaration を上で完了し、`None` を選んだ場合は具体的な理由を記載しました。
- [ ] I documented every new timeout, retry, pagination or result limit, truncation, fallback, or downgrade and its evidence. / 我已记录每项新增 timeout、retry、pagination 或结果限制、truncation、fallback 或 downgrade 及其证据。 / 新しい timeout、retry、pagination または result limit、truncation、fallback、downgrade とその証拠をすべて記載しました。
- [ ] I did not add refresh tokens, cookies, authorization codes, proxy credentials, private URLs, downloaded works, local state, or private API responses. / 我没有添加 refresh token、cookie、authorization code、代理凭据、私有 URL、下载作品、本地状态或私有 API 响应。 / refresh token、cookie、authorization code、proxy credential、private URL、downloaded work、local state、private API response を追加していません。
- [ ] I updated migration guidance for every breaking change. / 我已为每个破坏性变更更新迁移指引。 / すべての breaking change の migration guidance を更新しました。
