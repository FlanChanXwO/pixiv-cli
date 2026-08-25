# Final build script

## Command
```shell
cd /private/tmp/pixiv-cli-pixiv-e2e-fixes && sh scripts/build.sh
```

## Output
```text
go test ./internal/media/ugoira/staticlib -run ^TestCommittedManifestWhenPresent$ -count=1
ok  	github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira/staticlib	3.281s
GOOS=darwin GOARCH=arm64 go build -trimpath -o build/pixiv ./cmd/pixiv
built build/pixiv
```

Exit code: 0

Verdict: PASS
