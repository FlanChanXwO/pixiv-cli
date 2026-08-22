# Inspect built pixiv binary

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ls -l bin/pixiv && file bin/pixiv && shasum -a 256 bin/pixiv
```

## Output
```text
-rwxr-xr-x@ 1 flanchan  staff  46497906 Aug 21 19:18 bin/pixiv
bin/pixiv: Mach-O 64-bit executable arm64
9e8fe0f51e2f0564ca0b55ff654a8dbb67bbefcaba6e0a16551c20113aae3263  bin/pixiv
```

Exit code: 0

Verdict: PASS
