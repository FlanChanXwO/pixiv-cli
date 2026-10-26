---
name: pixiv-cli-code-commenting
description: In pixiv-cli, write and review package documentation, doc comments, field and concurrency comments, and numbered workflow stages. Use when an implementation changes a contract or non-obvious invariant, a package is created or its responsibility changes, a multi-stage function needs navigation, or existing comments are noisy, stale, or in the wrong language. Preserve compiler/tool directives. Do not require comments for every function or statement.
---

# pixiv-cli Code Commenting

Expose information that names and structure cannot express clearly: caller contracts, intent, constraints, invariants, and meaningful workflow stages. Optimize understanding, not comment count or code length.

## Repository context

Read root `AGENTS.md` and the affected owner before editing. This checked-in skill is self-contained; it does not require the personal `code-commenting` skill. Review caller contracts across CLI, SDK, and MCP without duplicating them in every layer. Keep machine-output, credential, transaction outcome, and Rust/cgo ownership comments precise; an ordinary prose cleanup must not change build directives or the cgo preamble. Use [pixiv-cli-test](../pixiv-cli-test/SKILL.md) for verification scope and [pixiv-cli-develop](../pixiv-cli-develop/SKILL.md) for any authorized structural change.

## Editing workflow

1. Read the affected symbol, enclosing flow, nearby comments, and applicable repository conventions. Inspect callers or tests when a claimed contract is unclear; do not infer thread safety, atomicity, retry safety, or ownership from a name.
2. Identify the missing information before adding prose. Keep accurate contract and intent comments, correct stale ones, and remove narration that adds no information. Leaving self-explanatory code uncommented is a valid outcome.
3. Choose the nearest useful location: API documentation for a caller contract, a type/field comment for an invariant or unit, an inline comment for a surprising choice, or numbered comments for a meaningful multi-stage flow.
4. Keep edits within the requested scope. A comment review is not permission to rename APIs, extract helpers, redesign modules, add tests, or introduce dependencies. Report structural or behavioral defects separately unless their correction is authorized.
5. Re-read comments against the final code and applicable documentation output. Confirm stage order, symbol references, factual guarantees, and protected directives. Report uncertain claims rather than documenting them as facts.

## Language and syntax

- Write comments in `sdk/`, including doc comments, in English; pkg.go.dev displays them. Write comments everywhere else (`cmd/`, `internal/`, `scripts/`, `.github/`, `frontend/`) in Simplified Chinese. When editing a comment in the wrong language, rewrite it in the required one; bulk translation of untouched code belongs to a dedicated comment task. This skill's English text does not change that rule.
- Program output (logs, error strings, CLI output, MCP result text) stays English regardless of the comment language.
- Use natural sentences in the chosen language. In Chinese prose keep protocol values, identifiers, and quoted error text in English, but do not mix in translatable English terms.
- Preserve identifier spelling, protocol values, units, URLs, issue references, and required documentation tags.

## Package documentation and file starts

Write package documentation in exactly one file per package, normally `doc.go` or the entry file `name/name.go`, directly above the `package` clause. Follow the standard library: the first sentence starts with `Package x` and says what the package does; after an empty `//` line, describe scope, what it does not own, and where related code lives. Do not record authors, dates, or history.

- A package with several files or several main types adds a "package composition" paragraph that lists the main types and entry points as doc links (`[Client]`, `[Session]`) so godoc navigates to them and gopls flags stale links after a rename.
- A `main` package's documentation starts with the command name, for example `pixiv 命令启动 pixiv-cli。`
- Do not start any other Go file with a comment. A comment attached to `package` merges into package documentation, and a detached file header goes stale after refactors. Choose a file name that tells the reader its contents; a file that needs a comment to explain itself usually needs a rename or a different split. Report that rather than renaming during a comment-only task.
- A `//go:build` constraint stays on the first line, followed by a blank line before package documentation or `package`. Keep cgo preambles and `//go:embed`/`//go:generate` directives where they are. Generated files keep the generator's header.
- `_test.go` files, formats without comments, `testdata/`, and `third_party/` take no file description. Shell, PowerShell, and `.cmd` scripts (after the shebang), workflow YAML (before `name:`), and frontend HTML/JS/CSS start with one comment line stating their purpose.

```go
// Package pixiv provides a Go client for the Pixiv App API: artworks, novels, users, bookmarks, and comments.
//
// [Client] carries every endpoint and is built by [New]. Upstream protocol details live in internal/pixivapi.
package pixiv
```

## Documentation comments

Document every exported type, function, method, constant, and variable, in `internal/` as well as `sdk/`. Start with the identifier and follow it directly with what it does or guarantees; state side effects, errors, cancellation, concurrency, ownership, and zero/nil/absent semantics when they apply. Avoid boilerplate headings and parameter lists that only repeat the signature. Document a private function only when its name and signature leave out a precondition, an ordering requirement, or a non-obvious trade-off.

Do not write definition sentences ("X 是……", "X is a ...") or open with "该函数", "此方法", or "用于". For example, a useful contract states an ownership rule:

```go
// Snapshot 返回当前设置的副本，调用方修改结果不影响 store。
```

Use that wording only when the implementation actually provides a copy. Avoid vacuous prose such as `Snapshot 返回快照`, and do not promise guarantees that the code or accepted contract does not establish: thread safety, atomicity, idempotency, or retry safety.

Give types their meaning and the invariants callers must keep. Comment a field only when its name and type leave out its unit, the meaning of zero or absence, optionality, the lock that guards it, or its ownership and lifetime; put a short note at the end of the field's line.

```go
type Budget struct {
	mu   sync.Mutex
	used int64 // 已占用字节，由 mu 保护
	max  int64
}
```

Keep a shared contract in its authoritative location and link or name it where helpful; do not copy long explanations into every caller.

## Comments for readers with basic Go

- Next to `select`, channel closing, `sync/atomic`, lock ordering, or a goroutine hand-off, add one line explaining why the code is written that way.
- For a state machine, account pool, OAuth flow, MCP session, or similarly complex module, explain the overall model once in the package or type documentation: the states, who owns what, and the order of events. Local comments then do not repeat it.

## Numbered workflow comments

Use `// 1. ...`, `// 2. ...`, and `// 3. ...` when stable, meaningful phases make an orchestration function easier to scan. Number phases, not statements; there is no mandatory number of stages, comments, or function length.

A useful outline reveals sequencing constraints rather than translating helper names. For a workflow that actually enforces these boundaries:

```go
// 1. 先校验整批记录，任何一条都还不可见。
// 2. 在事务锁内重新检查 revision，避免丢失更新。
// 3. 发布整批；提交后的清理失败按“已提交”报告。
```

Place each comment immediately before its actual block, not as a detached table of contents. Use the native comment syntax in languages other than Go.

- Keep stages at one abstraction level with parallel action wording and sequential numbering. Renumber after reordering, insertion, or deletion.
- Prefer a flat sequence. Nested numbering is justified only by a real nested workflow that benefits from navigation, not by nested `if` statements.
- Leave trivial getters, setters, forwarding wrappers, obvious short flows, assignments, and ordinary error checks unnumbered. If helper names already tell the whole story, omit the redundant outline.
- Keep an independent intent comment next to a subtle operation inside a stage; not every explanation needs a number.
- A difficult outline may reveal mixed responsibilities, but do not split code merely to shorten functions or obtain neat numbering. Preserve a coherent local flow unless an in-scope extraction reduces the reader's total effort.

## Intent, constraints, and temporary work

Explain why the obvious alternative is wrong, why ordering matters, which invariant later code depends on, or which external constraint requires a workaround. State uncertainty honestly and keep sensitive inputs out of examples and links.

```go
// 排行位置在两次请求之间会变化，用稳定的作品 ID 作键。
key := strconv.FormatInt(artwork.ID, 10)
```

The comment `把 ID 转成字符串` adds no information to that statement.

For a timeout, retry, limit, or fallback, identify its actual requirement, platform constraint, established protocol, or demonstrated failure. A comment does not justify an arbitrary restriction or a hidden fallback. Explain the relevant trigger and successful-path impact without inventing measurements or evidence.

Describe current behavior, limits, and reasons; never goal, task, work-item, or design-document numbers, or progress such as "implemented in a later stage". State an unsupported capability directly ("尚不支持 X"). Mark a temporary implementation as `shortcut: <限制>，<替换条件>`, with the replacement written as a condition. Make TODO/FIXME comments actionable with the specific issue or removal condition when known. Preserve real references; do not invent issue IDs, owners, deadlines, or speculative future work. Prefer `上游 v1 API 下线后删除这条兼容路径` over `以后再修`. Do not cite design documents, section numbers such as `§28`, or requirement IDs that are not in the repository; state the reason itself.

## Protected comments and behavior

Treat compiler, build, generator, linter, type-checker, and tooling directives as executable inputs, not expendable prose. Examples include Go build/embed/generate directives, cgo preambles, shell shebangs, type-checker directives, and snapshot or documentation-test markers. Preserve required position, spacing, and scope. Keep license notices and generated-file provenance intact; change a generator's source rather than its output when applicable.

Ordinary prose-only edits need a diff/doc/formatter check appropriate to the repository, not a new behavioral test per comment or function. A directive, doctest, reflected docstring, generated schema description, or other consumed comment can affect behavior; inspect its consumer and run the relevant existing check. Any actual behavior change follows the project's test-first contract. Do not create a scanner, new test framework, or exact-comment wording test merely to enforce this style.

## Readability review and completion

Check whether each edited comment adds a contract, reason, invariant, or navigation cue. Remove line-by-line narration, comments on every assignment or `if err != nil`, decorative banners, repeated function names, and unsupported guarantees; preserve meaningful explanations and required legal/tooling text.

Keep clear names, explicit data flow, and cohesive responsibilities ahead of prose. A short expression is not inherently clearer, and extracting many tiny helpers may make a flow harder to follow. Do not replace good structure with explanations or force unrelated refactoring to reduce comments.

Finish when affected contracts are accurate, each touched package has one package documentation, touched comments use the required language, helpful comments remain close to their code, numbering follows actual phases, and required document/tool checks have been observed. No comment-density target, per-function quota, or new-test count defines success.

For Go-specific details, consult [Go Doc Comments](https://go.dev/doc/comment) and [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) when needed. Repository behavior and verified evidence still determine what a comment may claim.
