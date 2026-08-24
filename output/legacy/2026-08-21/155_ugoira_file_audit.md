# Audit downloaded Pixiv ugoira file signature and extension

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && find downloads/ugoira -type f -print -exec /usr/bin/file --brief --mime-type {} \; -exec /usr/bin/file --brief {} \;
```

## Output
```text
downloads/ugoira/148767248 - 沖縄風ビーチのミク＆テト版「風」グラビア娘/Boisei属シバフたぬき - 沖縄風ビーチのミク＆テト版「風」グラビア娘_148767248.apng
image/png
PNG image data, 600 x 600, 8-bit/color RGBA, non-interlaced
```

Exit code: 0

Verdict: PASS
