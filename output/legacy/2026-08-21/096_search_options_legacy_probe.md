# Probe legacy search-options command and preserve actual incompatibility

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv search-options miku --json --proxy http://127.0.0.1:7890
```

## Output
```text
error: unknown command "search-options" for "pixiv"
```

Exit code: 1

Verdict: FAIL
