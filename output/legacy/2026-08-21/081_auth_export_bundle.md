# Export one local Pixiv account to a private temporary bundle

## Command
```shell
rm -f /private/tmp/pixiv-cli-e2e-shell-20260821/credentials/pixiv-auth.bundle && pixiv auth export 25649510 --output /private/tmp/pixiv-cli-e2e-shell-20260821/credentials/pixiv-auth.bundle && /bin/ls -lO /private/tmp/pixiv-cli-e2e-shell-20260821/credentials/pixiv-auth.bundle
```

## Output
```text
output: /private/tmp/pixiv-cli-e2e-shell-20260821/credentials/pixiv-auth.bundle
accounts: 1
-rw-------@ 1 flanchan  wheel  - 242 Aug 23 04:26 /private/tmp/pixiv-cli-e2e-shell-20260821/credentials/pixiv-auth.bundle
```

Exit code: 0

Verdict: PASS
