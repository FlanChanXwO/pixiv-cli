# Pixiv CLI リファレンス

[English](../en/cli-reference.md) | [简体中文](../zh-CN/cli-reference.md) | 日本語 | [プロジェクトホーム](../../README.ja.md)

これは `pixiv` command の完全な契約です。インストール、認証、command、flag、設定、環境変数、匿名
fallback、更新を扱います。SDK と MCP の詳細は重複させず、[関連ドキュメント](#関連ドキュメント)へ
案内します。

ユーザーに影響する変更は[バージョン別 changelog](../../changelog/README.md)に記録されます。

[GitHub Releases ページ]: https://github.com/FlanChanXwO/pixiv-cli/releases

## インストール

> **Release の状態**：対応 binary の Ed25519 public key、key ID、fingerprint は
> [`internal/bootstrap/release_trust.go`](../../internal/bootstrap/release_trust.go) に commit されています。
> public source/tap repository、保護された `release` Environment、分離された credential は設定済みです。
> v0.4.4 は 6 platform archive、checksum、署名付き manifest を含む公開 GitHub Release として公開されています。
> GitHub Release と tap は独立した公開物です。現在の状態は公式の [GitHub Releases ページ]と
> `brew info FlanChanXwO/tap/pixiv-cli` で確認してください。将来の version も同じ tag、署名、asset、
> Homebrew gate を通過するまでは信頼済み download source とみなしません。

### 公式インストールスクリプト

最新 stable Release をユーザー単位で導入する 2 種類の bootstrap script を提供します：

```bash
# Linux/macOS
curl -fsSLo /tmp/pixiv-install.sh https://github.com/FlanChanXwO/pixiv-cli/releases/latest/download/install.sh
sh /tmp/pixiv-install.sh --add-to-path
```

```bat
rem Windows Command Prompt。PowerShell は不要です
curl.exe -fsSLo "%TEMP%\pixiv-install.cmd" https://raw.githubusercontent.com/FlanChanXwO/pixiv-cli/main/scripts/install.cmd
call "%TEMP%\pixiv-install.cmd" --add-to-path
```

`install.sh` は Linux/macOS AMD64・ARM64 に対応し、既定では `$HOME/.local/bin` にインストールします。
`install.cmd` は Windows AMD64・ARM64 に対応し、既定では `%LOCALAPPDATA%\Programs\pixiv` を使います。
どちらも公式 latest stable Release から `checksums.txt` と一致する archive だけを取得し、展開前に
SHA-256 を検証し、staged binary の動作を確認してから `pixiv` を置換します。`--install-dir DIR` は保存先、
`--no-path` は profile/registry を変更しない指定です。Unix の `--add-to-path` は `$HOME/.local/bin` のみ、
Windows では現在のユーザーの `Path` だけを更新します。root/admin 権限を要求せず、前提ツールを勝手に
インストールせず、Pixiv credential を読み取らず、OS の reputation warning を回避しません。

Linux Release asset は glibc 2.35 以降を必要とします。release、native-evidence、packaged-smoke job は
両方の Linux architecture を Ubuntu 22.04 上で build し、GNU version requirement が `GLIBC_2.35` を
超える ELF を拒否します。installer の binary preflight は既存の install を置き換える前に loader failure を
明示します。

初回 bootstrap の script には Ed25519 verifier を埋め込めません。SHA-256 は破損・取り違えを検出しますが、
真正性は HTTPS と公式 GitHub repository/Release account に依存します。実行前に script を確認してください。
導入後の `pixiv update` は binary に埋め込まれた Ed25519 trust root を使用します。

正式版の installer には静的な Release-source list が埋め込まれます。権威ある `checksums.txt` は常に GitHub HTTPS から直接取得し、無料候補は対応する platform archive の probe にだけ使用します。候補の checksum は直取得した内容と byte 単位で一致する必要があり、install 前にも archive の SHA-256 を検証します。list を remote から取得することはなく、署名済み Release とともにだけ更新されます。

公式 installer は user ごとの on-demand `pixiv://` handler を初期化し、Homebrew も `post_install` で同じ処理をします。warning は検証済み binary の install 成功と desktop integration の未完了を意味し、最初の通常 browser `pixiv auth login` が再試行します。手動展開 archive には hook がありません。desktop Linux は `xdg-mime` と `gio` が必要で、headless Linux は relay server のみ対応します。

### source から build

```bash
sh scripts/build.sh
```

対応 source build には Go `1.26.3`、`CGO_ENABLED=1`、対象 platform の C linker、対応する Rust ugoira
staticlib が必要です。出力は `build/pixiv` または `build/pixiv.exe` です。Windows では Git Bash、MSYS2、
または WSL から実行します。

working tree には darwin/linux/windows × amd64/arm64 の runner 検証済み staticlib と、同一 source の
`manifest.json` が含まれます。`scripts/build.sh` は source digest、target/path、library の SHA-256 を
検証します。詳細は[開発フロー](../maintainers/development.md#rust-ugoira-staticlib)を参照してください。

### Go install

公式 tag 公開後は正確な tag を指定します：

```bash
go install github.com/FlanChanXwO/pixiv-cli/cmd/pixiv@vX.Y.Z
```

ローカル Go toolchain、cgo、C linker、対象 staticlib は引き続き必要です。branch name ではなく、常に
公開済みの正確な tag を使用してください。

### Homebrew

```bash
brew install FlanChanXwO/tap/pixiv-cli
```

将来の beta/pre-release channel：

```bash
brew install FlanChanXwO/tap/pixiv-cli-beta
```

両 formula は同じ `pixiv` binary を配置するため競合します。macOS/Linux の検証済み Release asset だけを
取得し、runtime の `ffmpeg` dependency は追加しません。

GitHub Release と public tap は別々の公開 channel です。現在の状態は
`brew info FlanChanXwO/tap/pixiv-cli` と [GitHub Releases ページ]で確認し、この reference の固定 version に
依存しないでください。

### 直接ダウンロード

Release は darwin、linux、windows の amd64/arm64 向けに
`pixiv-cli_<version>_<os>_<arch>.tar.gz`（Windows は `.zip`）を生成し、`checksums.txt` と Ed25519 署名付き
`checksums.json` を添付します。公開済みの v0.4.4 は 6 種類すべてを含みます。

現在 Apple notarization と Windows Authenticode はありません。macOS Gatekeeper や Windows SmartScreen が
警告する場合があります。公式 GitHub Release からだけ取得し、version、checksum、signature note を確認し、
出所不明 asset の警告を回避しないでください。

## refresh token の取得

`PIXIV_REFRESH_TOKEN` は Pixiv App API OAuth の raw refresh token でなければなりません。
`refresh_token=...`、`PHPSESSID`、`device_token` などの Web Cookie は常に拒否され、抽出・変換されません。

推奨フローは browser OAuth login です：

```bash
pixiv auth login
```

| 段階 | 動作 |
| --- | --- |
| 初期化 | PKCE verifier/challenge と OAuth state を生成し、local loopback HTTP server を起動します。 |
| Browser | macOS、desktop Linux、Windows では on-demand の current-user `pixiv://` callback helper を初期化して default browser を開き、既存 Pixiv session を再利用できます。`--no-open` では login URL と local page だけを表示します。 |
| Callback | この試行の loopback callback、一時 helper からの hand-off、terminal paste、local page form だけを受理します。helper の hand-off 後は、OAuth exchange 完了時に default browser が local の最終 success/failure page を開きます。戻らない場合は callback URL、`pixiv://...` URL、Pixiv relay URL、raw authorization code を貼り付けられます。 |
| 検証 | local callback はこの試行の state と一致する必要があります。Pixiv が state を返さない場合だけ、公式 callback URL と `pixiv://account/login` を明示 fallback として使えます。 |
| 保存 | refresh/access token は表示せず、refresh token を UID ごとに `auth.json` へ保存します。Unix-like は parent `0700`・file `0600`、Windows は既存 ACL を維持します。 |

handler は persistent ですが OS が `pixiv://` を開くときだけ動作します。macOS は `PixivCLIURLHandler.app`、Windows は current-user protocol association、desktop Linux は XDG desktop entry を使い、previous handler を private に記録します。active local loopback bridge が常に優先されます。ない場合は allowlist の正確な `pixiv://account/login` だけが remote relay に送られ、他の `pixiv://` URL は previous handler に定向します。`removing `login_relay_target_url` from private `config.toml`` は pixiv-cli がまだ default のときだけ previous association を復元し、後からの user 変更は上書きしません。binary を削除済みなら private `~/.pixiv-cli/url-handler/handler-manifest.json` の記録を使い、OS の association UI で復元してから manifest を削除します。cookie、storage、history、session、tab、traffic は読まず、managed Chromium、DevTools/CDP、browser-state scan も起動しません。

macOS で接管前の `pixiv://` handler がなかった場合、`removing `login_relay_target_url` from private `config.toml`` は pixiv-cli を default のまま黙って残さず error を返します。先に macOS の association UI で希望する handler を選び、もう一度 `unset` を実行してください。manifest だけを削除し、その後の user 選択は上書きしません。

GUI のない SSH server では listener を loopback に保ち、local machine から転送できる未使用の固定 port を
選びます。まず server で実行します：

```bash
pixiv auth login --no-open --addr 127.0.0.1:41871
```

次に local machine の別 terminal で実行します：

```bash
ssh -N -L 41871:127.0.0.1:41871 USER@SERVER
```

local browser で `http://127.0.0.1:41871/` を開きます。この tunnel は server の loopback listener
だけに接続し、callback port を公開しません。この forwarded page で login URL を開き、Pixiv relay または最終 callback を送信します。検証済み relay は同じ local browser で続行し、最終 callback は tunnel 経由で server listener へ POST されます。browser machine に pixiv の install は不要です。別案として interactive SSH terminal を使い、最終 callback URL、`pixiv://` URL、relay URL、raw authorization code を元の `auth login` prompt に貼り付けられます。login listener を public interface に bind しないでください。`--addr` は意図的に loopback address だけを受理します。

### Cross-machine callback relay

server が account を保存し browser が macOS、Windows、desktop Linux にある場合、server には
`login_relay_public_url`、`login_relay_listen_addr`、hidden input の `login_relay_secret` を、browser machine
には同じ secret と `login_relay_target_url` を設定します。server の `pixiv auth login` は Pixiv URL を表示して
1 回の認証済み callback を待つだけで、user が browser machine で URL を開きます。server は remote browser を
起動しません。`--relay-public-url`、`--relay-listen-addr`、`--relay-tls-cert-file`、`--relay-tls-key-file` は
1 回の server login だけを override します。role flag や常駐 relay process はありません。

認証済み callback が server に届くと、browser machine は設定済み relay URL 配下の one-time result page を開きます。
実際の OAuth exchange の完了を待ってから local login と同じ固定 success/failure page を表示し、その result URL に
callback や token は含まれません。

TLS PEM を直接指定するか、同一 host の reverse proxy で HTTPS を終端し listener を loopback に置きます。HTTP
も許可しますが、HTTP relay URL の設定時と HTTP server login のたびに callback と bearer secret が network で
観測・改変され得る warning を表示します。Web API、local subscription、通常 author work、empty success への
fallback はありません。
`pixiv config` が管理するのは download path、filename template、HTTPS proxy だけです。relay secret は private
`config.toml` を手動で管理し、`config get`、JSON、log、error、handler manifest には表示されません。

browser の system proxy は Go CLI に自動継承されません。必要なら先に設定します：

```bash
pixiv config set https_proxy http://127.0.0.1:7890
pixiv auth login --proxy http://127.0.0.1:7890
```

`--proxy URL` と `--no-proxy` は現在の command だけに適用され、同時使用できず、`config.toml` に保存されません。

HTTP(S) proxy を設定した場合、ugoira を含む `download` などの media resource transfer は意図的に HTTP/1.1 を使います。App API、OAuth、Web metadata request は通常どおり protocol negotiation を維持します。これは proxy 固有の HTTP/2 stream reset を回避するためであり、認証や選択した download quality は変わりません。
実 Pixiv login は OAuth Web flow に依存し、自動 test は fake OAuth server を使用します。

### 認証の import

direct import は raw Pixiv App OAuth refresh token を受け取ります：

```bash
pixiv auth import                         # TTY では非表示入力
printf '%s\n' 'YOUR_REFRESH_TOKEN' | pixiv auth import
pixiv auth import 'YOUR_REFRESH_TOKEN'    # argv/shell history に残ります
```

`pixiv auth import [REFRESH_TOKEN]` は raw token を App OAuth で検証し、Pixiv が返す UID を正として、rotation 後の refresh token を保存します。引数なしの TTY は非表示 prompt、非 TTY は stdin の opaque な 1 行を読み、末尾の LF または CRLF を 1 つだけ除きます。位置引数は process list、shell history、wrapper、監査 tool に記録される可能性があります。`--json` は secret を含まない account summary だけを変更します。`--proxy` と `--no-proxy` は direct validation にだけ適用され、併用できません。

direct import の成功時は `added uid:UID` または `updated uid:UID` を報告し、username がある場合は text に `username:NAME` も出力します。JSON は `{"user_id":12345678,"username":"display name","status":"added"}` のような secret-free account item 1 件だけです。`status` は `added` または `updated` で、default、token の有無、入力 token、rotation 後の token は公開しません。

export bundle を Pixiv に接続せず restore する例：

```bash
pixiv auth import --file account.pxauth
pixiv auth export --all | ssh trusted-host pixiv auth import --file -
```

`--file PATH` は file、`--file -` は stdin から完全な bundle を読みます。この mode は offline で token の検証や rotation を行わず、位置 token、`--proxy`、`--no-proxy` を拒否します。全 account を UID ごとに atomic merge し、既存 account は更新、新規 account は追加します。local default は保持し、default がない場合だけ bundle default を採用します。通常出力は入力 bundle 順に added/updated UID と最終 default を表示します。`--json` は `{"accounts":[{"user_id":12345678,"username":"display name","status":"added"}],"default_user_id":12345678}` を返し、account item は `user_id`、`username`、`status` だけを公開します。

### 認証の export と backup

```bash
pixiv auth export                         # default account の raw token
pixiv auth export 12345678                # 指定 account の raw token
pixiv auth export --all                   # stdout へ versioned all-account bundle
pixiv auth export 12345678 --output account.pxauth
pixiv auth export --all --output accounts.pxauth
pixiv auth export --all --output accounts.pxauth --force
```

`--output` なしで secret を stdout に書けるのは 2 形式だけです。default/UID export は保存済み raw token と改行 1 つ、`--all` は versioned JSON bundle だけを出力し、成功時の stderr は空です。export は local-only で、`auth.json` だけを読み、`PIXIV_REFRESH_TOKEN`、refresh、Pixiv network、auth/config mutation を使用せず、startup pending-update cleanup、automatic update、operation log も skip します。`--all` と UID は併用できず、`--force` には `--output` が必要です。JSON/proxy flag はありません。

`--output PATH` を指定すると single-account でも `--all` でも raw token ではなく bundle を書きます。既存 destination は既定で拒否し、明示的な `--force` だけが replacement を許可します。成功 stdout は output path と account count だけで secret を含みません。Unix-like の file は `0600` で、既存 parent の permission/ownership は変更しません。Windows は owner と protected DACL を明示し、current user、LocalSystem、builtin Administrators だけに full control を許可します。この Windows policy は CI tests で検証しますが、本 release の検証を実 Windows filesystem で実行したとは主張しません。

bundle は暗号化されていない secret の point-in-time backup で、live sync ではありません。元 token と同様に保護し、token rotation 後の古い bundle や別 machine の copy は stale になり得ます。strict versioned codec は unsupported schema/version、unknown/duplicate field、trailing JSON、duplicate/non-positive UID、empty token、bundle 内 account を指さない default UID を拒否します。top-level と account object の key は canonical な spelling/case と完全一致する必要があり、`Schema`、`Default_User_ID`、`User_ID`、`Refresh_Token` などの alias は canonical key と併存する場合も拒否します。

export selection/I/O failure は stdout に secret diagnostic を書きません。restore の atomic write failure では `LocalWriteCommitOutcome=not_committed` は replacement 前、`committed` は replacement 後の durability/cleanup failure、`unknown` は recovery で target state を確定できない状態です。後 2 つを rollback 成功として扱わず、store reload または手動確認が必要です。他の stdout/stderr、JSON、MCP result、log、error は refresh token を公開しません。persistent auth import/export MCP tool は追加せず、既存 session-scoped MCP 認証は不変です。

## CLI の使用方法

```bash
pixiv auth login

# 高度な/scripted setup（token を argv に置かない）
printf '%s\n' 'YOUR_REFRESH_TOKEN' | pixiv auth import
```

代表的な command：

```bash
pixiv auth list
pixiv auth use 12345678
pixiv auth check
pixiv auth refresh
pixiv config path
pixiv config get download_path
pixiv config set download_path ~/Downloads/pixiv
pixiv config unset https_proxy

pixiv version
pixiv version --json
pixiv --version
pixiv update --check
pixiv update --check --json

pixiv search "初音ミク"
pixiv search "初音ミク" --json
pixiv detail 123456
pixiv ranking --mode day
pixiv recommended all
pixiv download 123456 789012
```

永続的な application-managed data は current user の home directory 直下に保存されます。macOS/Linux は
`~/.pixiv-cli`、Windows は `%USERPROFILE%\.pixiv-cli` です。Pixiv UID ごとの `auth.json`、`config.toml`、
callback bridge state、daily log、Release-check cache、macOS callback helper がここに含まれます。既定は読みやすい
出力で、対応 command は `--json` を利用できます。`auth export` は意図的に JSON flag を持ちません。Cobra/pflag の
option は positional argument の前後どちらでも指定できます。

通常 command を初めて実行したとき、`config.toml` がなければ download、Web fallback、output、login、update の
common setting だけを含む baseline file を作成し、既存 file は決して上書きしません。proxy、logging のような advanced setting は明示設定まで省略されます。login timeout は `auth login --timeout` だけの明示 flag で、Premium-status cache は固定契約です。help、version、secret export、internal OAuth callback はこの file を作成しません。

### v0.8.0 data operation contract

非 mutating の data read、recommendation、feed、download はすべて `pixiv auth use` で選んだ local account を使います。手動の `[account_pool]` がある場合だけ、安全な operation は pool 内の local account を選べます。write、authentication、config は pool を使いません。data command は `--uid` と `--refresh-token` を拒否し、`PIXIV_REFRESH_TOKEN` を無視します。

stream 処理には `--ndjson` を使います。各行は stable string の `id`、`type`、`url` を持つ canonical Record で、残りの SDK field も保持します。`pixiv filter` は stdin の Record を読み、`download`、`bookmark add/remove`、`follow add/remove` は positional ID なしで Record を消費できます。action 成功時の stdout は空で、安全な diagnostic は stderr に出ます。`--on-error=skip|fail-fast` は malformed/incompatible stdin Record を制御します。`--json` と `--ndjson` は併用できません。

Ugoira download は `--ugoira-format gif|apng` を受け付け、既定は `gif` のままです。Ugoira の page selection または non-original quality と組み合わせることはできません。

### CLI command 一覧

| Command | Usage | 説明 |
| --- | --- | --- |
| `auth import` | `pixiv auth import [REFRESH_TOKEN] [--file PATH] [--json] [--proxy URL\|--no-proxy]` | direct input は rotation 後の token を検証・保存します。引数なし TTY は非表示、非 TTY は raw stdin です。`--file PATH|-` は offline atomic bundle restore となり、token/proxy input と競合します。 |
| `auth login` | `pixiv auth login [--json] [--no-open] [--addr 127.0.0.1:0] [--use] [--timeout DURATION] [--relay-public-url URL --relay-listen-addr ADDR] [--relay-tls-cert-file PATH --relay-tls-key-file PATH] [--proxy URL\|--no-proxy]` | 通常の loopback OAuth、または完全な server relay 設定時は 1 回の認証済み remote desktop callback を待ちます。UID ごとに保存し refresh token は表示しません。 |
| `auth list` | `pixiv auth list [--json]` | local account を一覧表示し、refresh token は表示しません。text の `*` は default、`✓`/`-` は local refresh token の保存/欠落を示すだけで、online validity を示しません。 |
| `auth export` | `pixiv auth export [UID] [--all] [--output PATH] [--force]` | default/指定/all account を local export します。`--output` なしは single raw token または `--all` bundle、指定時は private bundle と安全な stdout summary です。`--force` には `--output` が必要です。 |
| `auth use` | `pixiv auth use [UID] [--json]` | default account を設定し、TTY では対話選択できます。 |
| `auth remove` | `pixiv auth remove [UID] [--yes] [--json]` | account を削除します。default 削除後は残りの先頭を選択します。 |
| `auth check` | `pixiv auth check [UID] [--json] [--proxy URL\|--no-proxy]` | token を refresh して account を検証します。 |
| `auth refresh` | `pixiv auth refresh [UID] [--all] [--json] [--proxy URL\|--no-proxy]` | 保存済み default/指定 account の OAuth access token と rotation 後 refresh token を更新し、profile を強制取得して Pixiv Premium status cache を更新します。`--all` は全 account を更新し、JSON は常に `accounts` を返します。 |
| `config path` | `pixiv config path` | `config.toml` path を表示し、存在しなければ baseline file を作成します。 |
| `config get` | `pixiv config get KEY` | 有効な設定値を 1 件表示します。 |
| `config set` | `pixiv config set KEY [VALUE]` | 既知 key を書き込みます。`download_path`、`filename_template`、`https_proxy` だけを受け付けます。 |
| `config unset` | `pixiv config unset KEY` | 既知の設定キーを削除します。 |
| `version` | `pixiv version [--json]` | `version`、`commit`、`build_date` を表示します。root `pixiv --version` は version だけです。 |
| `update` | `pixiv update [--check] [--prerelease] [--proxy URL]` | 現在の install source に合わせて確認・更新します。`--json` は `--check` とだけ使用できます。 |
| `search` | `pixiv search [options] WORD` | イラストを検索します。 |
| `novel search` | `pixiv novel search [options] WORD` | 認証済み App API で小説を検索します。 |
| `search-options` | `pixiv search-options [options] WORD` | keyword に対する App API 制作ツール候補を表示します。認証が必要です。 |
| `detail` | `pixiv detail [options] ILLUST_ID_OR_URL` | 作品 ID または対応 Pixiv 作品 URL の詳細を表示します。 |
| `ranking` | `pixiv ranking [options]` | イラストランキングを表示します。 |
| `recommended` | `pixiv recommended all\|illust\|manga\|novel\|user [--page N --limit N --json]` | 指定 kind の認証済みおすすめを表示します。`all` は 4 種類を順に返します。 |
| `feed following` | `pixiv feed following --type illust\|novel [--restrict public\|private --page N --limit N --json\|--ndjson]` | follow 中 user の新作を読みます。 |
| `feed latest` | `pixiv feed latest --type illust\|manga\|novel [--page N --limit N --json\|--ndjson]` | App の latest works feed を読みます。 |
| `mypixiv users` | `pixiv mypixiv users [--page N --limit N --json\|--ndjson]` | 選択 account の MyPixiv user を一覧します。 |
| `mypixiv works` | `pixiv mypixiv works [USER_ID] --type illust\|manga\|novel [--page N --limit N --json\|--ndjson]` | MyPixiv works を一覧します。USER_ID 省略時は `illust` または `novel` だけです。 |
| `filter` | `pixiv filter [--id ID --type TYPE --tag TAG --min-views N --min-pages N --on-error skip\|fail-fast]` | stdin の canonical Record NDJSON を filter します。 |
| `user search` | `pixiv user search WORD [--page N --limit N --json]` | ユーザーを検索します。JSON/text は公式 App 検索か匿名の関連作品作者 fallback かを示します。 |
| `user detail` | `pixiv user detail USER_ID [--json]` | ユーザーの完全な公開 profile を表示します。 |
| `user artworks` | `pixiv user artworks [USER_ID] [--type TYPE --page N --limit N]` | 作品一覧。UID 省略時は現在の認証ユーザーです。 |
| `user bookmarks` | `pixiv user bookmarks [USER_ID] [--restrict public\|private --tag TAG --page N --limit N]` | visibility/tag で絞った bookmark 一覧。 |
| `user following` | `pixiv user following [USER_ID] [--restrict public\|private --page N --limit N]` | follow 一覧。 |
| `bookmark add` | `pixiv bookmark add ILLUST_ID [--restrict public\|private --tag TAG...]` | bookmark を追加します。`--tag` は反復可能です。 |
| `bookmark remove` | `pixiv bookmark remove ILLUST_ID` | bookmark を削除します。visibility/tag flag はありません。 |
| `follow add` | `pixiv follow add USER_ID [--restrict public\|private]` | 指定 visibility で follow します。 |
| `follow remove` | `pixiv follow remove USER_ID` | unfollow します。visibility flag はありません。 |
| `download` | `pixiv download [options] SRC...` | artwork PID/URL、許可された CDN URL、または対応 user URL の全 visual works を download します。 |
| `mcp` | `pixiv mcp [--proxy URL\|--no-proxy]` | MCP stdio server を起動します。 |

download filename は template と URL 由来 extension の cross-platform 不正文字を正規化します。ASCII control
character と Windows が拒否する末尾 dot/space も除去します。extension は upstream URL 由来のままで、
allowlist、MIME 推測、暗黙置換は行いません。

### `auth login` flag

| Flag | Default | 説明 |
| --- | --- | --- |
| `--json` | `false` | 保存結果を JSON で表示し、token は表示しません。 |
| `--no-open` | `false` | browser を自動起動・観察せず、login URL と loopback page だけを表示します。 |
| `--addr` | `127.0.0.1:0` | loopback listen address。port `0` は自動割当です。 |
| `--use` | `false` | 成功後に default account にします。default 不在時は自動で default になります。 |
| `--timeout` | `0` | login 待機時間。`0` は CLI が deadline を追加しません。 |
| `--relay-public-url` | config | この server relay の public HTTP(S) base URL。 |
| `--relay-listen-addr` | config | この server relay の listen host:port。 |
| `--relay-tls-cert-file` / `--relay-tls-key-file` | config | direct TLS の PEM pair。両方必要です。未指定で HTTPS public URL を使う場合は same-host reverse proxy と loopback listener が必要です。 |
| `--proxy URL` / `--no-proxy` | empty | この token exchange だけの proxy override で、保存されません。 |

### Data command flag

| Command | Flag | Default | 説明 |
| --- | --- | --- | --- |
| `search` | `--search-by` | `tag-partial` | 検索フィールド: `tag-partial`、`tag-exact`、`title-caption`、または App OAuth 専用の `tag-title-caption`（タグ、タイトル、キャプション）。 |
| `novel search` | `--search-by` | `tag-partial` | 検索フィールド: `tag-partial`、`tag-exact`、`title-caption`。 |
| `search`、`novel search` | `--sort` | `date_desc` | sort order: `date_desc` または `date_asc`。 |
| `search` | `--period` | empty | quick time range: `day`、`week`、`month`、`half-year`、`year`。省略時は期間指定なし。`--start-date` / `--end-date` と併用不可。 |
| `novel search` | `--period` | empty | time range: `day`、`week`、`month`。省略時は期間指定なし。 |
| `search` | `--start-date` / `--end-date` | empty | 包含境界の `YYYY-MM-DD` date。どちらか一方でも指定でき、両方ある場合 start は end より後にできません。`--period` と併用不可。 |
| `search`、`novel search` | `--rating` | `all` | `sfw`, `r18`, `r18g`, `mature`, `all`。 |
| `search` | `--type` | `all` | `all`, `illust-and-ugoira`, `illust`, `manga`, `ugoira`。 |
| `search` | `--ai-mode` | `all` | `all`, `exclude`, `only`。Pixiv `AIType==2` が AI 生成です。 |
| `search` | `--aspect-ratio` | `all` | `all`, `landscape`, `portrait`, `square`。 |
| `search` | `--resolution` | `all` | `all`, `high`, `medium`, `low`。両辺がそれぞれ `>=3000`, `1000..2999`, `<=999`。 |
| `search` | `--draw-tool` | empty | upstream の正確な制作ツール名。認証済み `search-options` で取得します。 |
| `search` | `--bookmark-min` / `--bookmark-max` | empty | 包含境界の非負 public bookmark 数。App OAuth と有効な Pixiv Premium 会員資格が必要で、`min` は `max` を超えられません。保存済み account では cached self-profile を search 前に確認し、非 Premium は local で block します。 |
| `novel search` | `--min-text-length` | `0` | 本文の最小文字数。`0` は下限を無効にします。 |
| `novel search` | `--max-text-length` | `0` | 本文の最大文字数。`0` は上限を無効にし、非ゼロの下限より小さくできません。 |
| `novel search` | `--original-only` | `false` | Pixiv がオリジナルと表示した小説だけを残します。 |
| list commands | `--limit` | one upstream batch | 最大件数。省略時は one upstream batch、`0` は next batch がなくなるまで取得します。 |
| list commands | `--page` | empty | 1-based logical page。正数 `--limit` が必要です。 |
| `ranking` | `--mode` | `day` | `day`、`day_male`、`day_female`、`week`、`week_original`、`week_rookie`、`month`、`day_manga`、`week_manga`、`month_manga`、`week_rookie_manga`、`day_r18`、`day_male_r18`、`day_female_r18`、`week_r18`、`week_r18g`。後半の 9 mode は認証が必要です。 |
| `ranking` | `--date` | empty | 通常 `YYYY-MM-DD`。 |
| `recommended KIND` | `--page`, `--limit` | per-stream | `all` でも 4 stream を独立に pagination します。 |
| `download` | `--pages` | empty | 1-based のページ指定（例: `1,3-5`、閉区間・重複排除・自然順）。省略時は全ページ。存在しないページは明示失敗します。 |
| `download` | `--quality` | `original` | 静止画品質: `original`、`regular`（長辺 1200）、`small`（長辺 540）、`thumb`（250×250 中央 crop）、`mini`（48×48 中央 crop）。Ugoira は original 以外の quality または pages 指定を unsupported として拒否します。 |
| `download` | `--download-path` | env/config/`./downloads` | download directory。他の command では受け付けません。 |
| `download` | `--filename-template` | env/config/`{author} - {title}_{id}` | filename template。placeholder は `{id}`、`{title}`、`{author}`。他の command では受け付けません。 |
| `download` | `--concurrency` | `0`（automatic） | download worker 数。`0` は `2 × GOMAXPROCS`、正数はそのまま使います。 |
| `user artworks` | `--type` | `illust` | Pixiv 作品種別: `illust`、`manga`、`ugoira`。 |
| `user bookmarks` | `--restrict` | `public` | `public` または `private`。 |
| `user bookmarks` | `--tag` | empty | 正確な bookmark tag filter。 |
| `user following` | `--restrict` | `public` | follow visibility。 |
| `bookmark add` | `--restrict` | `public` | 新規 bookmark visibility。 |
| `bookmark add` | `--tag` | empty | 反復可能な bookmark tag。 |
| `follow add` | `--restrict` | `public` | 新規 follow visibility。 |
| `detail` | `ILLUST_ID_OR_URL` | required | 正の artwork ID または対応 Pixiv artwork URL。 |
| `download` | `SRC...` | required | artwork PID、artwork URL、許可された CDN resource URL、または対応 user profile/artworks URL。CDN file は URL filename を使い、page/派生 quality/custom artwork template は使えません。 |

refresh token がある `search` は常に App API を使います。App は解像度、縦横比、tool、content type、
`ai-mode=exclude` を処理し、rating と `ai-mode=only` は App result batch ごとに適用されます。App failure は
Web に fallback しません。filter は opaque cursor に binding され、別条件へ再利用できません。ローカル filter が
連続 empty upstream batch を飛ばす場合、CLI/MCP は最初の非 empty 論理 batch か upstream 終端まで続けます。
正数 `--limit`/`--page` は filter 後の論理結果を跨 batch で埋め、`--limit 0` は全件走査、`--limit` なしでも
先頭 empty batch は skip します。App は明示 date と Pixiv Premium 限定の bookmark-count の境界も処理します。bookmark count は like-count
フィールドではなく、like と表記してはいけません。作品 JSON/text は
`https://www.pixiv.net/artworks/{id}` を先頭フィールド/先頭行として含めます。

bookmark-count bound では、保存済み account の fixed 24-hour self-profile Premium cache を再利用します。cache miss/expiry 時は search 前に profile を取得し、非 Premium なら Pixiv search endpoint に届く前に失敗します。
OAuth token とこの status を強制更新するには `pixiv auth refresh [UID]`（または `--all`）を使います。直接 SDK access token を渡す構築には
検証可能な local account identity がないため、この saved-account precheck は利用できません。

### イラストタグ検索の構文

認証済み App API で、イラスト `search` がタグ mode を選んだ場合に検証済みです。boolean tag filter には
`tag-exact` を使用します。`tagA tagB` は両方の完全な tag が必要な AND、`tagA OR tagB` はどちらかの完全な
tag でよい OR です。`OR` は大文字で指定します。文字列 `AND` は検証済みの演算子ではないため、二つの tag は
空白で区切ります。

既定の `tag-partial` も検証済みの大文字 `OR` 構文を受け付けますが、各語は曖昧な tag 条件です。結果を厳密な
exact-tag AND と説明してはいけません。部分 tag、alias、翻訳 tag に一致し、入力した完全な label が表示されない
場合があります。`title-caption` と App OAuth 専用の `tag-title-caption` には boolean tag の契約がありません。大文字の literal `OR` tag/keyword を
escape する構文は未検証なので、厳密な query ではその token を避けて exact tag を使用してください。

`novel search` は App 専用です。App request が表すのは keyword target、日付順、期間だけで、rating、本文長、
original-only は各 batch の安定した response field で検証されます。必須 field がなければ無言の非一致ではなく
typed upstream-response failure になります。filter が batch を飛ばす場合にも logical `--page`/`--limit` の意味は
変わりません。小説 JSON には `https://www.pixiv.net/novel/show.php?id={id}`、`x_restrict`、`text_length`、
`is_original` が含まれます。

認証済みの `detail`、pages、ugoira metadata は App API のみを使います。App のページ数不一致またはページ資源不足は明示的に失敗し、匿名 Web request は行いません。認証済み ugoira は original ZIP が得られないとき、検証済み App medium ZIP を直接 download します。冪等 App JSON read だけは、最初の 429 に有効な `Retry-After` がある場合に command context 配下で一度だけ待機・再試行します。header 不正/欠落、二度目の 429、write、resource download は replay しません。
`detail --json` は Pixiv の raw HTML `caption` を保持し、通常の `detail` 出力は安全な plain text に変換します。作品 list output には caption を含めません。

`detail` は正の artwork ID、または HTTPS の `pixiv.net`/`www.pixiv.net` にある `/artworks/{id}` URL を受け付けます。locale、query、fragment は許可されます。user、novel、short-link、FANBOX、Pixivision、Sketch、legacy、そのほかの URL は受け付けません。

`download` は許可された CDN URL に加えて `/users/{id}` と `/users/{id}/artworks` を受け付けます。user URL は `illust`、`manga`、`ugoira` を全 page にわたり download し、novel は対象外です。App OAuth が必須で、匿名 Web fallback は使いません。URL はローカルでのみ parse され、HTML 取得や redirect follow は行いません。1 件の失敗後も他の作品を続行し、cancel は直ちに停止します。download は ETag/Last-Modified metadata を `.pixiv-cache` に保存し、validator が一致する partial だけを `If-Range` で安全に再開して atomic に publish します。`download` は action です。成功時の stdout は空で、安全な failure は stderr に出し、完了できない場合は non-zero で終了します。download history や cross-run deduplication はありません。

### 共通 flag

| Flag | 適用先 | Default | 説明 |
| --- | --- | --- | --- |
| `--ndjson` | data list/read command | `false` | streaming filter/action 用に canonical Record を 1 行ずつ出力します。`--json` とは併用不可です。 |
| `--json` | safe data read、auth summary、`version`、`update --check` | `false` | command が対応する場合に complete result document を出力します。download/write action は success report を出しません。 |
| `--proxy URL` | network command と `mcp` | `https_proxy`/`HTTPS_PROXY`、`config.toml`、または empty | この command だけ HTTP(S) proxy を使います。`auth import --file` では不可です。 |
| `--no-proxy` | `--proxy` と同じ | empty | この command の HTTP(S) proxy を解除します。`--proxy` や bundle restore とは併用不可です。 |

### CLI が管理する `config` alias

`pixiv config get/set/unset` が受け付ける alias は**この 3 つだけ**です。ほかの runtime setting は private `config.toml` を手動で管理します。CLI に generic setting editor はありません。

| KEY | Type | Default | 説明 |
| --- | --- | --- | --- |
| `download_path` | string | `./downloads` | download directory。 |
| `filename_template` | string | `{author} - {title}_{id}` | filename template。 |
| `https_proxy` | string | empty | HTTP(S) proxy。lowercase `https_proxy` environment variable が優先。 |

手動 TOML には `[account_pool]`、`[web]`、`[login]`、`[logging]`、`[update]` などを置けます。`config.toml` に refresh token を書かないでください。account-pool state は last UID/freeze information だけで、token は保存しません。

### 環境変数

| Variable | Default | 説明 |
| --- | --- | --- |
| `PIXIV_REFRESH_TOKEN` | empty | 対応する public SDK/MCP runtime の credential input。CLI data command は意図的に無視します。 |
| `PIXIV_LOG_LEVEL` | empty | `log_level` を上書き。 |
| `DOWNLOAD_PATH` | `./downloads` | download directory。 |
| `FILENAME_TEMPLATE` | `{author} - {title}_{id}` | filename template。 |
| `https_proxy` / `HTTPS_PROXY` | empty | HTTP(S) proxy。lowercase が優先。 |

CLI data command は `pixiv auth use` の local default account、または手動 `[account_pool]` を選びます。credential-selection flag と `PIXIV_REFRESH_TOKEN` は読みません。

設定優先順：CLI flag > environment > `config.toml` > built-in default。proxy override は保存されません。

### 匿名 Web fallback

token source がなく `web_fallback_enabled=true` の場合、CLI の `search`、`detail`、`ranking`、`download`
は Pixiv Web/ajax API を利用できます。refresh token がある場合は App API を優先し、invalid token、network、
server error を自動 fallback しません。

- 匿名 `search` は Web が確実に表現できる filter だけを使用します。AI は返却 field で判定します。
- `rating=r18|r18g|mature`、`--search-by tag-title-caption`、または bookmark-count filter は request 前に認証要求として失敗し、空結果に見せません。bookmark-count filter には Pixiv Premium も必要です。`all` は匿名で見える範囲です。
- `search-options` は App 専用です。Cookie を読み取らず、refresh token を Web session に変換しません。
- `novel search` は App 専用で、refresh token がなければ認証要求として失敗します。
- 拡張 ranking mode（`day_manga`、`week_manga`、`month_manga`、`week_rookie_manga`、`day_r18`、
  `day_male_r18`、`day_female_r18`、`week_r18`、`week_r18g`）は認証が必要で、匿名の日次ランキングに fallback しません。
- 認証済み `user search` は公式 App user search を使用し、`source: "app_search"` を返します。匿名 fallback は work search の author を `userId` で dedupe し、`source: "related_illust_authors"` を返して username search ではないことを明示します。
- 静止画は `/ajax/illust/{id}/pages` の `original`、ugoira は `/ajax/illust/{id}/ugoira_meta` の
  `originalSrc` と frame を使い、対応 build は内蔵 Rust encoder で GIF/APNG を生成します。
- 専用 proxy env はなく、共通 `--proxy`、environment、config を使います。

invalid proxy URL は network 前に失敗し、diagnostic に userinfo、path、query を出しません。無効化：

```bash
# ~/.pixiv-cli/config.toml
[web]
fallback_enabled = false
```

## Version と更新

```bash
pixiv version
pixiv version --json
pixiv --version

pixiv update --check
pixiv update --check --json
pixiv update --check --prerelease
pixiv update --proxy http://127.0.0.1:7890
```

development build は `dev` と表示し self-update を拒否します。公式 install は Homebrew stable/beta、
`go install`、Release binary を検出します。Homebrew channel 切替失敗時は元 formula の復元を明示的に試み、
両方の結果を報告します。`go install` は正確な Release tag、Release binary は Ed25519 署名 checksum manifest
と archive SHA-256 を検証し、`pixiv version --json` preflight 後に atomic replace します。

明示的な `--proxy`、設定済み `https_proxy`、または `HTTPS_PROXY` がない場合、Release binary update は内蔵 source list を並行 probe します。API 対応候補は GitHub Releases API に、archive 対応候補は署名 manifest、checksum、platform archive に使用されます。最初の有効 response が優先 route となり、asset download が失敗すると残る宣言済み route を各一回だけ静かに試します。全て失敗した場合だけ error に各 route を表示します。候補は canonical Release URL、SemVer 選択、Ed25519 verification、SHA-256 verification を変更しません。自動 update notification は API 対応候補だけを使い、既存の 3 秒総制限と 24 時間 cache を維持します。

update check は canonical SemVer tag だけを選びます。対象 channel に non-SemVer published Release があれば、
古い version へ暗黙に戻らず fail closed します。private key は保護された `release` Environment と管理された
recovery copy にだけ存在します。

通常 command 成功後の best-effort stable check は MCP、help、`version`、`update`、全 `auth export`、`auth import --file`、development
build を除外し、user cache ごとに 24 時間 1 回、3 秒上限です。新 version や check failure は stderr のみに
出し、business exit code、JSON stdout、MCP stdout を汚しません。無効化：

```bash
# ~/.pixiv-cli/config.toml
[update]
check_enabled = false
```

## 関連ドキュメント

- [Go SDK](sdk.md)：public client、model、pagination、resource、typed error。
- [MCP tools](mcp-tools.md)：tool、input schema、output、stdio behavior。
- [アーキテクチャ（中国語・簡体字）](../maintainers/architecture.md)：package responsibility と runtime flow。
- [開発フロー（中国語・簡体字）](../maintainers/development.md)：environment、test、build、release gate。
- [Agent skill](../../skills/pixiv-cli/SKILL.md)：インストール済み CLI を安全に操作するための指示。
