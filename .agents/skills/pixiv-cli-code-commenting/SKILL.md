---
name: pixiv-cli-code-commenting
description: In pixiv-cli, write and review intent-focused code comments, API documentation, docstrings, and numbered workflow stages. Use when an implementation changes a contract or non-obvious invariant, a multi-stage function needs navigation, or existing comments are noisy or stale. Keep natural-language choice neutral and preserve compiler/tool directives. Do not require comments for every function or statement.
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

- Accept either English or Chinese source comments; this skill's English instructions do not select a comment language. Follow the explicit task preference, otherwise the surrounding code and intended readers. Keep a coherent language within a comment; do not bulk-translate unrelated code or duplicate every comment bilingually.
- Preserve identifier spelling, protocol values, units, URLs, issue references, and required documentation tags. Natural-language freedom does not waive language-specific syntax.
- Use the repository's existing formatter and documentation conventions. For Go, place a doc comment directly before its declaration, normally start with the declared identifier (or `Package name` for a package), and use complete sentences. Chinese prose can follow the unchanged identifier.
- Document newly added or changed exported Go declarations and caller-visible field semantics. A concise, accurate contract is enough; avoid boilerplate headings and parameter lists that only repeat the signature. A private helper does not automatically need a doc comment.

## Documentation comments

Describe only the caller-relevant facts that apply: observable behavior, preconditions, zero/nil/absent semantics, units or formats, ordering, side effects, ownership/lifetime, concurrency, cancellation, and error or partial-commit outcomes.

Give important structs and domain types their meaning and invariants. Document fields individually only when their meaning is not already clear from the type contract and names. Keep implementation walkthroughs out of API documentation.

For example, a useful contract states an ownership rule:

```go
// Snapshot returns a copy of the current settings.
// The caller may modify the result without changing the store.
```

Use that wording only when the implementation actually provides a copy. Avoid vacuous prose such as `Snapshot returns a snapshot`, and do not promise guarantees that the code or accepted contract does not establish.

Keep a shared contract in its authoritative location and link or name it where helpful; do not copy long explanations into every caller.

## Numbered workflow comments

Use `// 1. ...`, `// 2. ...`, and `// 3. ...` when stable, meaningful phases make an orchestration function easier to scan. Number phases, not statements; there is no mandatory number of stages, comments, or function length.

A useful outline reveals sequencing constraints rather than translating helper names. For a workflow that actually enforces these boundaries:

```go
// 1. Validate the complete batch before any record becomes visible.
// 2. Recheck revisions under the transaction lock to avoid lost updates.
// 3. Publish the batch; report post-commit cleanup errors as committed.
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
// Ranking positions change between requests; key by the stable artwork ID.
key := strconv.FormatInt(artwork.ID, 10)
```

The comment `Convert the ID to a string` adds no information to that statement.

For a timeout, retry, limit, or fallback, identify its actual requirement, platform constraint, established protocol, or demonstrated failure. A comment does not justify an arbitrary restriction or a hidden fallback. Explain the relevant trigger and successful-path impact without inventing measurements or evidence.

Make TODO/FIXME comments actionable with the specific issue or removal condition when known. Preserve real references; do not invent issue IDs, owners, deadlines, or speculative future work. Prefer `Remove this compatibility path when the upstream v1 API is retired` over `fix this later`.

## Protected comments and behavior

Treat compiler, build, generator, linter, type-checker, and tooling directives as executable inputs, not expendable prose. Examples include Go build/embed/generate directives, cgo preambles, shell shebangs, type-checker directives, and snapshot or documentation-test markers. Preserve required position, spacing, and scope. Keep license notices and generated-file provenance intact; change a generator's source rather than its output when applicable.

Ordinary prose-only edits need a diff/doc/formatter check appropriate to the repository, not a new behavioral test per comment or function. A directive, doctest, reflected docstring, generated schema description, or other consumed comment can affect behavior; inspect its consumer and run the relevant existing check. Any actual behavior change follows the project's test-first contract. Do not create a scanner, new test framework, or exact-comment wording test merely to enforce this style.

## Readability review and completion

Check whether each edited comment adds a contract, reason, invariant, or navigation cue. Remove line-by-line narration, decorative banners, repeated function names, and unsupported guarantees; preserve meaningful explanations and required legal/tooling text.

Keep clear names, explicit data flow, and cohesive responsibilities ahead of prose. A short expression is not inherently clearer, and extracting many tiny helpers may make a flow harder to follow. Do not replace good structure with explanations or force unrelated refactoring to reduce comments.

Finish when affected contracts are accurate, helpful comments remain close to their code, numbering follows actual phases, and required document/tool checks have been observed. No comment-density target, per-function quota, mandatory translation, or new-test count defines success.

For Go-specific details, consult [Go Doc Comments](https://go.dev/doc/comment) and [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) when needed. Repository behavior and verified evidence still determine what a comment may claim.
