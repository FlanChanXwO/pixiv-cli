# Test Ownership: Same-Package Exception List

> 本清单是 `legacyInternalTestAllowlist`（已于 Goal 4 / T10 随 `internal/architecture` 一起删除）的**显式替代**。它不再是允许自动检查的机器读取白名单，而是供人工 review 核对的披露清单：每个目录的测试留在生产包内（`package X` 而非 `package X_test`）必须有理由。
>
> 规则：新 owner 测试一律使用 external test package（`X_test`）；只有下列目录被允许 same-package。新增例外必须在本文件登记 permanent 理由或 temporary 删除条件，否则视为违规。
>
> 依据：Goal 3 findings R21/R29；Goal 4 `tasks.md` T15；Goal 4 `architecture-replacement-matrix.md` 规则 #12（TestExternalTestPolicy）。

## Permanent（必须观察未导出内部态）

| 目录 | 必须 same-package 的理由 |
| --- | --- |
| `internal/browsercookies/chromium` | 测试直接构造 provider 并注入加密 key override（`newProvider` + 私钥链状态），观察未导出的 cookie 记录解密路径与 profile 发现逻辑；导出这些状态只为测试会污染生产 API。 |
| `internal/browsercookies/firefox` | 测试观察未导出的 profile 发现（`profiles.ini` 解析）、cookie database 路径解析与记录布局；这些是 provider 内部实现细节，不构成公开契约。 |
| `internal/browsercookies/safari` | 测试直接调用未导出的 `parseBinaryCookies`，断言 binarycookies 记录布局（`c.domain`/`c.httpOnly` 等字段级断言）；记录布局是内部格式，只对同一包测试开放。 |
| `internal/browsercookies/secret` | 测试构造 `SecretService{command: ...}` 注入未导出字段，并断言未导出的哨兵错误（`ErrSecretService`/`ErrInvalidItem`）与命令输出脱敏行为；这些是错误契约的一部分但无需导出。 |
| `internal/update/installer` | 测试注入未导出的 `assetURLValidator` seam（`installer.assetURLValidator = ...`）与校验和验证函数（`verifyArchiveChecksum`），观察安装器资源选择与恢复路径；seam 是测试替身而非公开 API。 |
| `internal/update/release` | `source_route_test.go` 为 same-package：测试观察未导出的 source route 选择与 canonical API URL 缓存状态（`newGitHubReleaseClient` 的内部路由）；`releases_*_test.go` 已用 external package，本目录是混合包。 |
| `scripts/internal/browsernativeevidence` | 测试观察未导出的环境探测（`currentGoEnvironment`、`setEnvironment`、`firefoxDataRootFor`、`findRepositoryRoot`）并注入合成 Firefox cookie 种子；探测逻辑是脚本内部实现。 |
| `scripts/internal/changescope` | 测试直接调用未导出的路径解析（`splitNULPaths`、`docsOnlyPaths`）与变更范围判定，观察 docs-only 过滤语义；范围逻辑是脚本内部实现。 |
| `scripts/internal/homebrewformula` | 测试直接调用未导出的 formula 渲染与版本校验（`renderFormula`、`validateFormulaVersion`、`checkDynamicVersionNeeds`），断言 Homebrew formula 生成契约；渲染细节不导出。 |
| `scripts/internal/licensebundle` | 测试观察未导出的 `defaultBundleFileOps`、`generateFromTargetMetadata` 与 license 文本归一化（`normalizeLicenseText`），并注入 fake cargo metadata；生成管线细节不导出。 |
| `scripts/internal/linuxabi` | 测试直接调用未导出的 glibc 版本解析与 ABI 比较（`parseGLIBCVersion`、`checkImportedSymbols`、`got.compare`），断言 libc 符号扫描；解析细节是脚本内部实现。 |
| `scripts/internal/nativeevidence` | 测试直接调用未导出的 mutation 工具（`mutation.apply`、`test.mutate`）注入并回滚 evidence 文件变更，观察 go/ast 解析与哈希校验；mutation 是内部测试设施。 |
| `scripts/internal/prepublishhomebrew` | 测试注入未导出的 `test.mutate` 与 CI 变更检测，观察 formula 生成与发布前检查的失败路径。 |
| `scripts/internal/publicapi` | 测试观察未导出的解析器对 `unexported`/`hidden` 符号的处理与 golden 比较逻辑，使用 `writeFixture` 生成 fixture；API 表面提取细节不导出。 |
| `scripts/internal/releaseassets` | 测试注入未导出的 `injectReleaseSources`/`injectWindowsReleaseSources`，观察 asset 归档命名（`archiveName`）与 checksums 生成。 |
| `scripts/internal/releasenotes` | 测试观察未导出的 GitHub client 调用映射（`client.firstMergedPullRequest`、`client.pullRequestsForCommit`），注入 fake client 断言 release-note 提取；client 接口是内部 seam。 |
| `scripts/internal/releaseworkflow` | 测试注入未导出的 `mutation.mutate`/`mutation.run` 观察 workflow 状态机与 git 环境注入；mutation 是内部测试设施。 |
| `scripts/internal/understandgraph` | 测试注入未导出的 `test.mutate`/`test.setup` 观察图构建与依赖关系输出。 |

## Temporary（绑定删除条件）

当前**无** temporary 项。历史 allowlist 曾把 `internal/update/release` 的 `releases_*` 部分标为迁移目标，该部分已实际迁至 external package（`package release_test`），仅 `source_route_test.go` 因观察未导出路由状态保留 same-package（已列入 Permanent 表）。

## 变更纪律

- 新增目录进入本清单：必须在 review 中说明观察到的**具体未导出符号**，并确认不能通过导出最小接口替代。
- 从本清单移除：必须给出删除 task 与测试迁移证据（external package 编译通过 + 覆盖不变）。
- 本清单不接受「迁移期」「将来」这类无期限表述。
