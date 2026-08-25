# Final code review scope check

## Command
```shell
cd /private/tmp/pixiv-cli-pixiv-e2e-fixes && git diff --name-only origin/main...HEAD | wc -l && git diff --numstat origin/main...HEAD | awk {add+=
```

## Output
```text
      23
awk: syntax error at source line 1
 context is
	 >>> {add+= <<< 
awk: illegal statement at source line 1
	missing }
```

Exit code: 2

Verdict: FAIL
