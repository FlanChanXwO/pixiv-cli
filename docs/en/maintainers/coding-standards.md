# Coding standards

English | [简体中文](../../zh-CN/maintainers/coding-standards.md) | [Documentation index](../../index.md)

This handbook is the single source of truth for how pixiv-cli Go code is written and reviewed. Maintenance skills describe the workflow and route here for the rules; [AGENTS.md](../../../AGENTS.md) routes tasks and lists hard constraints. Structure and ownership live in [architecture](architecture.md); toolchain, test layout, and release steps live in [development](development.md).

## Style baseline

- The [Google Go Style Guide](https://google.github.io/styleguide/go/) (Guide, Style Decisions, Best Practices) is the baseline. Where this handbook conflicts with it, this handbook wins.
- Use the Go version declared in `go.mod` and format with `gofmt`.
- Write code an engineer with basic Go can follow: clarity over brevity, no cleverness, no abstraction reserved for possible future needs.

## Naming and terminology

Name a package, file, type, function, or field for what it is and what it owns, not for its position in the code structure. `internal/` names may change freely; a public SDK rename is a breaking change listed in the changelog.

| Rule | Avoid | Prefer |
| --- | --- | --- |
| No vague names | `Deps`, `Dependencies`, `Data`, `Info`, `Facade`, `Manager`, `Helper`, `Util`, `Common`, `Base`, `Impl`, `Wrapper` | `pixivcmd.Runner`, `command.Env`, `session.Pixiv`; rename `Manager` for its actual job |
| No dependency bags | one `Dependencies` struct per command, filled by the root and passed down | `download.New(env command.Env, session *session.Pixiv)`; split an owner that needs too many dependencies |
| No function field behind a same-named forwarding method, no `Fn` suffix | `PromptSecretFn func(string) (string, error)` plus `func (d Data) PromptSecret` | `func (e Env) PromptSecret(message string) (string, error)`, or a concrete type or narrow interface for behavior that must vary |
| No context the package name already gives | `session.PixivSession`, `filename.FilenameData` | `session.Pixiv`, `filename.Fields` |
| One name per capability | `UsageError` and `Usage`, `PrintJSON` and `WriteJSON` side by side | one name on `command.Env` |

These rules apply to new code only (since 2026-10-26). Existing names are not renamed for them, and public SDK names are not broken again for them.

- No `Kind`, `Type` (as a catch-all discriminator), `Item`, `Object`, or `Misc` as a name or suffix. Use the specific role: `Phase`, `Mode`, `Source`, `Status`, `Registry`, `Store`. Keep upstream wire field names, such as Pixiv `type`, unchanged at the mapping layer.
- No abbreviations: write `Config`, `Manager`, `Service`, `Message`, `Request`, `Response`, `Temporary`, `Value`. Short names remain fine for locals used within a few lines (`ctx`, `err`, `ok`, `i`, `n`, `req`) and for one- or two-letter receivers. Keep initialisms such as `ID`, `URL`, and `HTTP` in full capitals.
- Package names are short lowercase nouns; avoid `util`, `common`, `base`, and `types`. Keep each directory name equal to its package name. Avoid names that collide with the public SDK packages, common standard-library packages (`runtime`, `log`, `context`), or another package imported by the same caller; needing an import alias signals a naming problem.

## Declaration placement and file names

| Scope of the declaration | Placement | Example |
| --- | --- | --- |
| Shared by several files in one package | A declaration file named for its subject | `sdk/pixiv/models.go` (public entities), `sdk/pixiv/artwork_wire.go` (artwork response shape), `sdk/pixiv/response.go` (response shapes shared across domains), `internal/storage/database/rows.go` (row structs), `errors.go` (error codes) |
| Serves one behavior | Beside the function that uses it, above it | `internal/mcpserver/pixiv/tools/users.go`: `userArtworksIn`; `internal/cli/internal/pixivcmd/list.go`: `ArtworkList` |
| Methods of a type (`String`, `Valid`, `MarshalJSON`, …) | Same file as the type | `sdk/resource.go`: `Resource.MarshalJSON` |

- Do not split one type per file.
- Do not create subjectless catch-all files such as `types.go` or `consts.go`.

| File case | Rule | Example |
| --- | --- | --- |
| Single-file package | `name/name.go` | `internal/cli/ranking/ranking.go` |
| Entry of a multi-file package | `name.go` holds the main entry: main type, constructor, or cobra command definition; other files are named by topic | `internal/cli/search/search.go` defines `pixiv search`, `filters.go` holds filtering |
| File that holds only package documentation | rename it to `doc.go`, or merge the documentation into the entry file | — |
| Entry file already covers several topics | split by topic and keep `name.go` as the entry | — |
| Any other file | name its content; avoid `support.go`, `deps.go`, `helpers.go`, `util.go`, `common.go`, `types.go` | `internal/cli/auth/` files named by responsibility |

`name.go` means "start reading this package here". It is not a storage room.

New code goes where its owner already lives, checked in this order: the directory of the one command it serves; the shared CLI package for that product or for all commands; the MCP product's `tools` package; the public SDK domain file, with upstream protocol detail in `internal/pixivapi` or `internal/fanboxapi`; otherwise a new package named for the responsibility. If the package, file, or type name no longer summarizes its contents, the code is in the wrong place. Never create `common`, `shared`, `util`, or `misc` locations.

## Named constants

Identifiers with business meaning use named constants instead of inline string literals; a fixed value set uses a named type so the compiler checks it.

| Category | Form | Placement |
| --- | --- | --- |
| Configuration key (alias) | `settings.Key` constant, for example `settings.KeyLogLevel` | `internal/config/settings/keys.go` |
| Enum value (log level, output format, restrict, content type, …) | named type plus constants, for example `settings.LogLevelDebug`, `pixiv.RestrictPublic` | same file as the type |
| Upstream API path, query/form parameter, header name | exported constant, for example `pixivapi.PathUserFollower`, `pixivapi.ParamUserID`; the package is under `internal/`, so module users cannot import it | `internal/pixivapi/api.go`, `internal/fanboxapi/api.go` |
| MCP tool name | unexported constant, for example `toolUserDetail` | same file as the tool registration |
| Database column referenced from Go | unexported constant, for example `colCredentialRevision` | `internal/storage/database/rows.go`, beside the row struct |
| JSON output key outside a struct tag, for example `illusts` | unexported constant | same file as the output code |
| Environment variable name | still declared by the `env` tag; define a constant when code must reference it separately | `keys.go` |

Meaningful values — lengths, limits, timeouts, intervals, exit codes, file names, addresses, service names — are named constants too. A contract-defined value lives in the package that enforces it. Define each value once; reference the constant or derive from it (`strconv.Itoa`) elsewhere. Values such as `0` and `1` with no business meaning, and arguments required by a standard-library contract such as `net.Dial("tcp", ...)`, need no name. The rule covers new and touched code; cleaning up every existing literal is separate work.

Literals remain acceptable for:

- struct tags, which Go requires to be literals;
- log messages, error text, and format strings;
- SQL migration files and SQL bodies in `Raw` queries;
- test inputs; an assertion on an identifier still references the production constant.

Configuration keys: the `alias` tags on `RuntimeConfig` fields remain the source of truth, and the constants in `keys.go` match them one to one. A table-driven test walks every `Key` constant through `SettingSpecByAlias`, because a typo otherwise becomes a runtime "unknown setting". Program code reads `RuntimeConfig` fields, such as `cfg.LogLevel`, and never alias strings. `Store.Get`, `Set`, and `Unset` serve the `config get/set/unset` commands, where the alias comes from the user.

```go
// internal/pixivapi/api.go

// App API 路径。
const (
	PathUserFollower  = "/v1/user/follower"
	PathUserFollowing = "/v1/user/following"
)

// 查询与表单参数名。
const (
	ParamUserID   = "user_id"
	ParamRestrict = "restrict"
)
```

```go
// sdk/pixiv/models.go

// Restrict is the visibility scope of a bookmark or following list.
type Restrict string

const (
	RestrictPublic  Restrict = "public"
	RestrictPrivate Restrict = "private"
)
```

## Boundary validation, no over-defensive code

Ask three questions about a structure: which caller would write more code without it, which concrete failure it prevents, and how many places a reader must visit to follow one call. No answer to the first two plus a non-zero third means remove it.

| ID | Criterion | Action |
| --- | --- | --- |
| P-a | Exported symbol referenced only by tests (public `sdk/` API excepted) | Make it unexported, or delete it and let the test use the production entry point |
| P-b | Package-level mutable variable replaced by an exported `SetXxx` in tests | Move it to an unexported struct field set by a same-package test |
| P-c | Package-level function that forwards to a default instance; `At`, `WithX`, or `WithFileStore` entry variants | Delete; callers use the single entry point |
| P-d | The same interface or mapping function written in several packages; the same method or mechanism written on several types, for example three credential-bearing account types each implementing `String`, `GoString`, and `Format` for redaction | Keep one copy, and move the mechanism onto the field type or value type it protects or describes, such as a credential field with a self-redacting type, so outer types stop repeating it |
| P-e | Parallel type layers, such as a DTO plus a model | Merge into one type |
| P-f | A home-grown substitute for the standard library or an existing dependency, such as `slog` or `cmp.Or` | Use the existing capability |
| P-g | Code or tooling with no production caller | Delete |
| P-h | Comment citing a document or section number that is not in the repository | State the reason, or delete the comment |
| P-i | Pass-through layer that only converts between structurally identical types or forwards a call unchanged | Delete; share one type or call directly |
| P-j | One concept split across tiny packages or files that must be read together | Merge them |
| P-k | Indirection reserved for imagined extension: an interface that copies one concrete type's method set in the same layer, a per-package copy of the same interface, a registry with one entry, an option with one value | Use the concrete type. A test substitute is not a second production implementation; exercise real boundaries with `httptest` or temporary directories. A consumer-owned narrow port that cuts a storage, network, or operating-system dependency, such as `session`'s `AccountService` or MCP `toolhost.SDKPorts`, is a real boundary and stays |
| P-l | Compatibility layer: old and new entry points, old-name aliases, source-compatible wrappers, branches kept for this project's older components, removed-setting tombstones, stale input aliases | Delete it and keep only the new path. Upstream format differences (Pixiv/FANBOX API, browser cookies) are not compatibility layers, and read logic that opens a user's existing database or config file stays |
| P-m | Over-defense: re-validating a precondition an internal caller guarantees; nil checks on fields or dependencies the constructor always sets; checks Go already makes, such as a nil map read or `len`/range on a nil slice; quietly degrading, returning a zero value, or swallowing an error on an impossible state; defensive copies without shared-mutation risk; `recover` "just in case" | Delete it. Keep the outermost check once. Let an internal invariant fail immediately with a clear error or panic instead of looking like success |

These criteria change structure, not user-visible features. Feature-scope decisions belong to the feature owner.

Validation stays at trust boundaries: user input, CLI and MCP arguments, upstream responses, files and database content, and environment variables. It also stays at the public SDK entry point, on credential copies that define ownership such as `RefreshTokenCopy`, and on concurrency and cancellation checks.

No internal compatibility: a change that supersedes an entry point removes the old one in the same change. Public SDK breaking changes are listed in the changelog and get no `Deprecated` transition.

Limits, timeouts, retries, truncation, and fallbacks exist only for a verified requirement, a platform constraint, an established contract, or a reproducible failure. State the trigger, cover it with the smallest evidence, and report failures instead of discarding valid data silently. Document an unsupported capability as unsupported.

```go
// sdk/resource.go: JSON encoding omits URL, request headers, and expiry metadata.

// MarshalJSON emits only the opaque reference and the credential requirement,
// never URL, request headers, or expiry metadata.
func (r Resource) MarshalJSON() ([]byte, error) {
	if r.Ref.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(struct {
		Ref                 ResourceRef `json:"ref"`
		RequiresCredentials bool        `json:"requires_credentials,omitempty"`
	}{r.Ref, r.RequiresCredentials})
}
```

```go
// internal/cli/internal/command/env.go：Env 由组合根构造，WrapUsage 一定存在；
// 判 nil 只会在装配写错时成立，却会把用法错误的退出码悄悄丢掉。
func (e Env) UsageError(err error) error { return e.WrapUsage(err) }
```

## Program output

Logs, error strings, CLI output, and MCP result text are English, independent of comment language. Machine-output channels, HTTP authorization, and truthful structured failures are hard constraints in [AGENTS.md](../../../AGENTS.md).

## Package documentation and comments

Write comments only for information names and structure cannot express: caller contracts, intent, constraints, invariants, ownership, and meaningful workflow stages. Keep a shared contract in its authoritative location and link to it where useful instead of repeating a long explanation in every caller.

### Package documentation

One file per package carries the package documentation, normally `doc.go` or the entry file `name/name.go`, directly above `package`. Follow the standard library:

- The first sentence starts with `Package x` and says what the package does. A `main` package starts with the command name instead, for example `pixiv 命令启动 pixiv-cli。`
- After a blank `//` line, describe scope, what the package does not own, and where related code lives. No authors, dates, or history.
- A package with several files or several main types adds a composition paragraph with doc links (`[Client]`, `[Session]`) to the main types and entry points. godoc navigates them, and gopls flags a stale link after a rename.
- No other Go file starts with a comment. A comment attached to `package` merges into the package documentation, and a detached file header goes stale after refactors. A file that needs a comment to explain itself needs a better name or split.

Directives and special files:

- `//go:build` stays on the first line, followed by a blank line before package documentation or `package`. cgo preambles and `//go:embed`/`//go:generate` keep their position.
- Generated files keep the generator's header.
- Keep license notices and generated-file provenance intact; when generated output needs a change, update its generator rather than editing the output by hand.
- `_test.go`, formats without comments, `testdata/`, and `third_party/` take no file description.
- Shell, PowerShell, and `.cmd` scripts describe their purpose on one line after the shebang; workflow YAML before `name:`; frontend HTML/JS/CSS at the top of the file.

```go
// Package pixiv provides a Go client for the Pixiv App API: artworks, novels, users, bookmarks, and comments.
//
// [Client] carries every endpoint and is built by [New]. Upstream protocol details live in internal/pixivapi.
package pixiv
```

```go
//go:build windows

package loginhelper
```

```sh
#!/bin/sh
# 为单个目标平台构建 pixiv 二进制，注入版本号，并对 Linux 产物运行 linuxabi 检查。
```

### Comment language

- `sdk/` comments, including doc comments, are English; pkg.go.dev displays them.
- Comments in `cmd/`, `internal/`, `scripts/`, `.github/`, and `frontend/` are Simplified Chinese. Rewrite an existing comment in the wrong language when you touch it; bulk translation of untouched code is separate work.
- Protocol fields, identifiers, and quoted error text stay English; do not mix translatable English terms into Chinese sentences.

### Doc comments

Document every exported type, function, method, constant, and variable, in `internal/` as well as `sdk/`. Start with the identifier, then say what it does or guarantees; include side effects, errors, cancellation, concurrency, ownership, and zero, nil, or absent meaning where they apply. Document a private function only when its name and signature leave out a precondition, an ordering requirement, or a non-obvious trade-off.

Avoid definition sentences ("X 是……") and openings such as "该函数", "此方法", or "用于":

| Avoid | Why | Prefer |
| --- | --- | --- |
| `// New 创建一个带 opaque FANBOX session 的账号值，并复制输入凭据。` | The first half repeats the name and signature | `// New 会复制 sessionID，调用方之后修改原切片不影响账号。` |
| `// Account 是一个已保存的 FANBOX 账号记录。session ID 是 opaque credential。` | Definition sentence; mixes English terms into Chinese | `// Account 对应本机保存的一个 FANBOX 账号；session ID 只能通过 SessionIDCopy 取得副本。` |
| `// FANBOX session 不与 Pixiv refresh token 共享类型或生命周期。` | Translated phrasing | `// FANBOX 的 session 和 Pixiv 的 refresh token 分开存放、分开轮换。` |
| `// String 返回字符串表示。` | No information | Delete it; when it redacts, say so: `// String 只输出账号摘要，日志和 %v 不会打印出凭据。` |

Types and fields: a type comment says what it represents and which invariants callers must keep. Comment a field only when name and type leave out the unit, the meaning of zero or absence, optionality, the guarding lock, or ownership and lifetime; a short note goes at the end of the field line.

```go
type Budget struct {
	mu   sync.Mutex
	used int64 // 已占用字节，由 mu 保护
	max  int64
}
```

### Numbered stages

When a function has several clear phases, put `// 1. ...`, `// 2. ...` before each block to name its purpose or ordering requirement. Number phases, not statements; simple functions, getters, and forwarding functions stay unnumbered.

- Keep stages at one abstraction level with parallel wording, and renumber after reordering.
- Put each comment immediately before its block, not as a detached table of contents.
- Prefer a flat sequence; nested numbering needs a real nested workflow.
- A difficult outline may reveal mixed responsibilities, but do not split code merely to shorten a function.

### Readable for engineers with basic Go

- Next to `select`, channel closing, `sync/atomic`, lock ordering, or a goroutine hand-off, add one line on why it is written that way.
- Explain a state machine, account pool, OAuth flow, or MCP session once in the package or type documentation: the states, who owns what, and the order of events. Local comments then do not repeat it.
- Do not narrate each assignment, `if err != nil`, or return, and do not add decorative separators.
- Do not promise guarantees the code does not provide, such as thread safety, atomicity, idempotency, or retry safety.

### Intent, constraints, and temporary work

- Explain why the obvious alternative is wrong, why ordering matters, which invariant later code relies on, or which external constraint requires a workaround. Keep the explanation beside the relevant code.
- For a limit, timeout, retry, or fallback, state the real requirement, platform constraint, established protocol, or demonstrated failure that triggers it, and the effect on the normal success path. Do not invent measurements or use a comment to justify an arbitrary restriction.
- Make TODO/FIXME comments actionable with a real issue or removal condition when known. Preserve real references; do not invent issue IDs, owners, deadlines, or speculative future work.
- Describe current behavior, limits, and reasons. Do not cite goals, tasks, work items, or design-document numbers, or describe implementation progress. State unsupported behavior directly. Mark temporary work as `shortcut: <limit>, <replacement condition>`, with a condition such as `replace when the upstream provides a stable ID`.

## Test submission rules

Permanent code and behavior tests cover only these long-lived contracts, with one representative case and necessary failure cases per contract. Reuse an existing check before writing a new one.

| Contract | Owning boundary |
| --- | --- |
| Credential privacy: account and Cookie values stay private under `%v`, `%+v`, `%#v`, and slog; logs omit query, token, Cookie, and signed URL; MCP output omits signed URLs and Cookie | Account and Cookie types, request logging, MCP output |
| Account storage: revision conflicts, transaction rollback, opening a v1.2.x database, file permissions; atomic replacement and file locking | `internal/storage/*` |
| A rotated token is persisted before the client is returned | `internal/pixivaccount` or `internal/session` |
| Malformed upstream responses return errors; cancellation propagates | `sdk/pixiv`, `sdk/fanbox` |
| Public snapshots: one full MCP `tools/list` golden; one CLI command's text, JSON, and NDJSON; usage-error exit code; stdio stdout carries only JSON-RPC; HTTP requires bearer | `internal/mcpserver`, `internal/cli` |
| Environment over file over default, including explicit empty values | `internal/config/settings` |
| A failed update verification preserves the installed binary | `internal/selfupdate` |

Retain tests for credential privacy, data integrity and persistence, public CLI/MCP/SDK output, cancellation and error propagation, and release-artifact correctness such as checksums and archive names. Assert each contract at its owning public boundary, and prefer checks that execute the behavior over string-only checks.

| Item | Cap |
| --- | --- |
| Non-exempt test files across the repository | 25 |
| Lines per test file, including imports, blank lines, and helpers | 200 |
| Lines per test function | 40 |
| Cases in one table | 8 |
| `testdata` | Old-version database samples and a few goldens |

- Count files for every platform and build tag, not only the current host.
- Use `httptest`, temporary directories, and real stores. Do not add test helper packages, fixture constructors, mock hierarchies, or production exports that exist for tests.
- Keep each assertion at its owning boundary; a CLI or MCP output test exercises presentation, not SDK mapping again.
- Keep CI workflows and steps unchanged during test cleanup. Preserve workflow-selected CI tests verbatim with their supporting code and fixtures; they are exempt from the file/function/case caps and `testdata` restriction. Broad `go test ./...` selectors do not exempt the whole repository. In a mixed file, keep the selected test, its comments, and required declarations exactly; other tests still follow this checklist. Recheck selectors before rewriting tests; report a selector with no matching test instead of claiming a pass.
- Preserve the CI-selected inventory exactly: whole packages `scripts/citest` (platform-smoke, container-smoke, and release runs), `scripts/release` (release workflow), `internal/cli/auth/loginhelper`, and `internal/media/ugoira/...`; named tests `cmd/pixiv/TestPixivBinaryPackagedSmoke` and `internal/storage/database/TestOpenOnWindowsAcceptsForwardSlashFileURI`. Recheck the names and package paths against current workflow selectors before test cleanup.
- Remove duplicate assertions, getter and pure-forwarding tests, tests of test helpers or test harnesses, and completed migration comparison/feature tests. Remove their orphan helpers, fixtures, and test-only production seams with them; preserve v1.2.x database-opening evidence. Real-environment tests are manual, not permanent tracked tests.
- Temporary investigation lives in ignored `tmp/test/_probe/`, so ordinary `go test ./...` does not discover it. Use synthetic credentials.
- A behavior change observes a failing test first, then passes, then keeps the regression. An import or toolchain failure is not Red. Pure restructuring uses before and after characterization.

Test layout, CI exemptions, and the live verification procedure: [development](development.md#tests).

## Applying and reviewing

- New files follow this handbook completely.
- A modified file brings its touched parts up to standard. Untouched code is not rewritten in the same change.
- Repo-wide catch-up is separate work.
- CI passing does not show that these rules were followed. Review the diff against this handbook.
