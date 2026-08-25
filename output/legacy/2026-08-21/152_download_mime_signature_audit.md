# Audit downloaded Pixiv file extensions and content signatures

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && for file in downloads/regular/*/* downloads/thumb/*/*; do printf "path=%s\n" "$file"; printf "extension=%s\n" "${file##*.}"; /usr/bin/file --brief --mime-type "$file"; /usr/bin/file --brief "$file"; done
```

## Output
```text
path=downloads/regular/148767434 - Vocaloid_ MILA THE JAGUAR 💚💜🎵/🎗TataBlue2004🧩 - Vocaloid_ MILA THE JAGUAR 💚💜🎵_148767434_p0.jpg
extension=jpg
image/jpeg
JPEG image data, JFIF standard 1.01, aspect ratio, density 1x1, segment length 16, baseline, precision 8, 900x1200, components 3
path=downloads/thumb/148767434 - Vocaloid_ MILA THE JAGUAR 💚💜🎵/🎗TataBlue2004🧩 - Vocaloid_ MILA THE JAGUAR 💚💜🎵_148767434_p0.jpg
extension=jpg
image/jpeg
JPEG image data, JFIF standard 1.01, aspect ratio, density 1x1, segment length 16, baseline, precision 8, 250x250, components 3
```

Exit code: 0

Verdict: PASS
