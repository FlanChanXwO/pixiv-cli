# Task 17 delivery correction

## Finding and repair

The first Task 17 push contained the final summaries but not the three download case directories. The final summary reported 95 cases while `git ls-tree` on the remote-aligned HEAD contained only 92 `report.md` files. The repository-wide `downloads/` ignore rule had silently excluded those case directories.

This correction tracks the nine missing text evidence files and narrows the ignore exception to `report.md`, `stdout.txt`, and `stderr.txt` inside structured download case directories. Real media, databases, and other arbitrary files remain ignored.

Before the correction:

```text
HEAD case reports: 92
working-tree case reports: 95
```

The first broad exception also failed the security review because it made an arbitrary `artifact.jpg` eligible for tracking. The focused ignore-contract check now returns:

```text
artifact_ignore_status=0
report_ignore_status=1
stdout_ignore_status=1
stderr_ignore_status=1
```

For `git check-ignore -q`, status 0 means ignored and status 1 means eligible for tracking. The result therefore preserves all three evidence files while continuing to ignore media.

## Regression command results

### Runner behavior

Command: `zsh scripts/e2evidence/run-case_test.zsh`

```text
/private/tmp/pixiv-evidence-runner-test.jsWGjq/run/cases/smoke/stream-separation
run-case behavior: PASS
```

Exit code: 0.

### Targeted repair packages

Command: `go test ./internal/services/pixiv/endpoint/user/detail ./internal/services/pixiv/endpoint/artwork/timeline ./internal/media/downloader ./internal/mcpserver/pixiv/tools/download ./sdk/pixiv`

```text
ok  internal/services/pixiv/endpoint/user/detail (cached)
ok  internal/services/pixiv/endpoint/artwork/timeline (cached)
ok  internal/media/downloader (cached)
?   internal/mcpserver/pixiv/tools/download [no test files]
ok  sdk/pixiv (cached)
```

Exit code: 0. Package names above are shortened from their common module prefix for readability.

### Full Go suite

Command: `go test ./...`

```text
All listed packages completed with `ok` or `[no test files]`.
The real binary E2E package completed successfully: e2e 33.917s.
No package emitted FAIL.
```

Exit code: 0.

### Vet

Command: `go vet ./...`

```text
(no output)
```

Exit code: 0.

### Standard build

Command: `sh scripts/build.sh`

```text
go test ./internal/media/ugoira/staticlib -run ^TestCommittedManifestWhenPresent$ -count=1
ok  github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira/staticlib 3.303s
GOOS=darwin GOARCH=arm64 go build -trimpath -o build/pixiv ./cmd/pixiv
built build/pixiv
```

Exit code: 0.

## Corrected evidence audit

An initial audit used the wrong short option for “files without matches”; it reported `cwd_errors=95` even though every report contained the required cwd. That audit result was rejected. The corrected command used `--files-without-match` and returned `cwd_errors=0`.

A later staged-content audit incorrectly passed `--cached` to `git grep`; Git rejected that unsupported option, so the pipeline-derived zero was also rejected. The corrected scan searched the indexed working tree with `pipefail` enabled and returned status 1, Git grep's documented “no matches” result.

```text
case_count=95
contract_errors=0
pass_count=82
fail_count=13
cwd_errors=0
secret_hits=0
root_files=0
legacy_count=180
tracked_download_case_files=9
tracked_media_or_state_files=0
```

Live temporary download signatures were also rechecked: regular and thumbnail files are JPEG, and the Ugoira APNG is PNG. Those media files remain under `/private/tmp` and are not tracked.

## Review conclusion

The broad ignore exception was the only new review finding (P2: accidental media/secret tracking risk) and is fixed in this correction. After the repair, the staged diff contains only the focused ignore rules, the nine missing text evidence files, and this report. No P0, P1, P2, or P3 finding remains.
