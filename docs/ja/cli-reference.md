# Pixiv CLI リファレンス

[English](../en/cli-reference.md) | [简体中文](../zh-CN/cli-reference.md) | 日本語 | [プロジェクトホーム](../../README.ja.md)

これは `pixiv` command の完全な契約です。インストール、認証、command、flag、設定、環境変数、匿名
fallback、更新を扱います。SDK と MCP の詳細は重複させず、[関連ドキュメント](#関連ドキュメント)へ
案内します。

> 独立した `pixiv filter` と `--ugoira-format` は削除されました。visual list は pipe 時に canonical NDJSON を自動出力します。作品は `--filter EXPR`（例: `bookmarkCount >= 5000 and xRestrict == 0`）で絞り込みます。download は `--ugoira-mode gif|apng|zip|frames`、`3-` page selection、`--archive`、`--write-metadata`、`--directory-template`、`--retries`、`--retry-delay` をサポートします。template は `{id}`、`{title}`、`{author}`、`{author_id}`、`{date}`、`{tags}`、`{num}` を使えます。proxy は `http`、`https`、`socks5`、`socks5h` を受け付けます。config は `directory_template` と `request_interval` を含み、`PIXIV_REQUEST_INTERVAL` または今回限りの `--sleep-request` で上書きできます。

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

公式 installer は user ごとの on-demand `pixiv://` handler を初期化し、Homebrew も `post_install` で同じ処理をします。warning は検証済み binary の install 成功と desktop integration の未完了を意味します。macOS と Windows では次の通常 `pixiv` command が初期化を再試行するため、手動展開 archive も最初の利用時に desktop integration を修復します。desktop Linux は `xdg-mime` と `gio` が必要で、headless Linux は desktop handler を登録せず relay server を実行できます。

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

`PIXIV_REFRESH_TOKEN` は Pixiv App API OAuth の raw refresh token です。

推奨フローは browser OAuth login です：

```bash
pixiv auth login
```

| 段階 | 動作 |
| --- | --- |
| 初期化 | PKCE verifier/challenge と OAuth state を生成し、local loopback HTTP server を起動します。 |
| Browser | macOS と Windows の通常 CLI 起動は current-user `pixiv://` callback helper を準備します。desktop Linux は interactive login 時に XDG handler を初期化します。CLI は default browser を開くため、既存の Pixiv session を再利用できます。`--no-open` では login URL と local page だけを表示します。 |
| Callback | この試行の loopback callback、一回限りの desktop hand-off、terminal paste、local page form を受理します。helper の hand-off 後は、OAuth exchange 完了時に default browser が local の最終 success/failure page を開きます。 |
| 検証 | local callback はこの試行の state と一致する必要があります。Pixiv が state を返さない場合だけ、公式 callback URL と `pixiv://account/login` を明示 fallback として使えます。 |
| 保存 | refresh/access token は表示せず、refresh token を UID ごとに local SQLite database へ保存します。Unix-like は parent `0700`・file `0600`、Windows は既存 ACL を維持します。 |

handler は persistent ですが OS が `pixiv://` を開くときだけ動作します。macOS は `PixivCLIURLHandler.app`、Windows は current-user protocol association、desktop Linux は XDG desktop entry を使い、previous handler を private に記録します。active local loopback bridge が常に優先されます。ない場合、`pixiv://account/login` は active な一回限りの desktop hand-off だけが受理します。`pixiv://account/remote-login` はその hand-off を開始します。その他の `pixiv://` URL は previous handler に委譲します。別の handler を選ぶ必要がある場合は、OS の association UI を使用してください。

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
だけに接続し、callback port を公開しません。manual page を到達可能にするだけで、browser-only machine の Pixiv 最終 `pixiv://` callback は受信できません。browser machine に pixiv-cli が install 済みなら、下記の一回限り desktop hand-off を使用してください。完全な最終 callback URL は元の `auth login` prompt に貼り付けることもできます。login listener を public interface に bind しないでください。`--addr` は意図的に loopback address だけを受理します。

### Cross-machine one-time hand-off relay

server に account を保存し、別の browser で認証する場合は server に `login_relay_public_url` と
`login_relay_listen_addr` を設定します。`pixiv auth login` は今回だけ有効な remote hand-off URL を表示します。
この URL を開くと `pixiv://account/remote-login` に直接 redirect され、pixiv-cli の session page、confirmation page、callback copy form は表示しません。

pixiv-cli を install 済みの desktop では local CLI がその一回の session を受け取り、OAuth URL を開始して結果の
callback を server へ返します。hand-off はその session だけ有効で、新しい hand-off は以前の local hand-off を置き換えます。desktop handler のない client ではこの relay flow を完了できないため、pixiv-cli を install 済みの desktop を使用してください。

relay は HTTP または HTTPS を使用できます。direct TLS には PEM pair を指定するか、同じ host の reverse proxy で
HTTPS を終端して listener を loopback に置きます。旧 `login_relay_secret` と `login_relay_target_url` の設定は静かに無視されます。
`pixiv auth devices` は削除されました。`pixiv config` が管理するのは download path、filename template、HTTPS proxy だけで、advanced relay setting は private `config.toml` に置きます。

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

`--output` なしで secret を stdout に書けるのは 2 形式だけです。default/UID export は保存済み raw token と改行 1 つ、`--all` は versioned JSON bundle だけを出力し、成功時の stderr は空です。export は local-only で、local SQLite database だけを読み、`PIXIV_REFRESH_TOKEN`、refresh、Pixiv network、auth/config mutation を使用せず、startup pending-update cleanup と automatic update も skip します。`--all` と UID は併用できず、`--force` には `--output` が必要です。JSON/proxy flag はありません。

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
`~/.pixiv-cli`、Windows は `%USERPROFILE%\.pixiv-cli` です。Pixiv UID ごとの `pixiv-cli.db`、`config.toml`、
callback bridge state、Release-check cache、macOS callback helper がここに含まれます。既定は読みやすい
出力で、対応 command は `--json` を利用できます。`auth export` は意図的に JSON flag を持ちません。Cobra/pflag の
option は positional argument の前後どちらでも指定できます。

通常 command を初めて実行したとき、`config.toml` がなければ download、output、login、update の
common setting だけを含む baseline file を作成し、既存 file は決して上書きしません。proxy のような advanced setting は明示設定まで省略されます。login timeout は `auth login --timeout` だけの明示 flag で、Premium-status cache は固定契約です。help、version、secret export、internal OAuth callback はこの file を作成しません。

### Data operation contract

account pool を無効にしている場合、非 mutating の data read、recommendation、timeline、download は `pixiv auth use` で選んだ local account を使います。`[account_pool]` に `enabled = true` を明示した場合だけ database-backed pool が有効になり、account 行の `schedulable` が参加可否を決めます。`strategy` の既定は `round_robin` で、`random` も使えます。`pixiv auth pool status|enable|disable` で状態を確認・変更できます。write、authentication、config は pool を使いません。data command は `--uid` と `--refresh-token` を拒否し、`PIXIV_REFRESH_TOKEN` を無視します。

visual list は pipe 時に canonical NDJSON を自動出力し、明示的な `--ndjson` も使えます。各行は stable string の `id`、`type`、`url` を持つ canonical Record です。`download`、`bookmark add/remove`、`follow add/remove` は positional ID なしで Record を消費できます。visual list と `download` は `--filter EXPR` で local artwork rule を適用します。`--on-error=skip|fail-fast` は malformed/incompatible stdin Record を制御し、`--json` と `--ndjson` は併用できません。

Ugoira download は `--ugoira-mode gif|apng|zip|frames` を受け付け、既定は `gif` です。Ugoira の page selection または non-original quality は明示的に失敗します。

### CLI command 一覧

| Command | Usage | 説明 |
| --- | --- | --- |
| `auth import` | `pixiv auth import [REFRESH_TOKEN] [--file PATH] [--json] [--proxy URL\|--no-proxy]` | direct input は rotation 後の token を検証・保存します。引数なし TTY は非表示、非 TTY は raw stdin です。`--file PATH|-` は offline atomic bundle restore となり、token/proxy input と競合します。 |
| `auth login` | `pixiv auth login [--json] [--no-open] [--addr 127.0.0.1:0] [--use] [--timeout DURATION] [--relay-public-url URL --relay-listen-addr ADDR] [--relay-tls-cert-file PATH --relay-tls-key-file PATH] [--proxy URL\|--no-proxy]` | 通常の loopback OAuth を使います。完全な server relay 設定がある場合は、install 済み desktop CLI handler を直接起動する一回限りの hand-off URL を表示します。UID ごとに保存し refresh token は表示しません。 |
| `auth list` | `pixiv auth list [--json]` | local account を一覧表示し、refresh token は表示しません。text の `*` は default、`✓`/`-` は local refresh token の保存/欠落を示すだけで、online validity を示しません。 |
| `auth pool` | `pixiv auth pool status [--json]`；`pixiv auth pool enable UID... [--all]`；`pixiv auth pool disable UID... [--all]` | secret を含まない database scheduling state を表示・変更します。`status` は `enabled`、`strategy`、`schedulable`、`frozen_until`、現在の `eligible` を表示し、enable/disable は全 UID を検証してから一括 commit します。 |
| `auth export` | `pixiv auth export [UID] [--all] [--output PATH] [--force]` | default/指定/all account を local export します。`--output` なしは single raw token または `--all` bundle、指定時は private bundle と安全な stdout summary です。`--force` には `--output` が必要です。 |
| `auth use` | `pixiv auth use [UID] [--json]` | default account を設定し、TTY では対話選択できます。 |
| `auth remove` | `pixiv auth remove [UID] [--yes] [--json]` | account を削除します。default 削除後は残りの先頭を選択します。 |
| `auth check` | `pixiv auth check [UID] [--json] [--proxy URL\|--no-proxy]` | token を refresh して account を検証します。 |
| `auth refresh` | `pixiv auth refresh [UID] [--all] [--json] [--proxy URL\|--no-proxy]` | 保存済み default/指定 account の OAuth access token と rotation 後 refresh token を更新し、profile を強制取得して Pixiv Premium status cache を更新します。`--all` は全 account を更新し、JSON は常に `accounts` を返します。 |
| `config path` | `pixiv config path` | `config.toml` path を表示し、存在しなければ baseline file を作成します。 |
| `config get` | `pixiv config get KEY` | 有効な設定値を 1 件表示します。 |
| `config set` | `pixiv config set KEY [VALUE]` | 既知 key を書き込みます。`account_pool_enabled`、`account_pool_strategy`、`download_path`、`filename_template`、`https_proxy` を受け付けます。 |
| `config unset` | `pixiv config unset KEY` | 既知の設定キーを削除します。 |
| `version` | `pixiv version [--json]` | `version`、`commit`、`build_date` を表示します。root `pixiv --version` は version だけです。 |
| `update` | `pixiv update [--check] [--prerelease] [--proxy URL]` | 現在の install source に合わせて確認・更新します。`--json` は `--check` とだけ使用できます。 |
| `search` | `pixiv search [options] WORD` | イラストを検索します。 |
| `novel search` | `pixiv novel search [options] WORD` | 認証済み App API で小説を検索します。 |
| `detail` | `pixiv detail [options] ILLUST_ID_OR_URL` | 作品 ID または対応 Pixiv 作品 URL の詳細を表示します。 |
| `ranking` | `pixiv ranking [options]` | イラストランキングを表示します。 |
| `recommended` | `pixiv recommended all\|illust\|manga\|novel\|user [--page N --limit N --json]` | 指定 kind の認証済みおすすめを表示します。`all` は 4 種類を順に返します。 |
| `timeline following` | `pixiv timeline following --type illust\|novel [--restrict public\|private --page N --limit N --json\|--ndjson]` | follow 中 user の新作を読みます。 |
| `timeline latest` | `pixiv timeline latest --type illust\|manga\|novel [--page N --limit N --json\|--ndjson]` | App の最新作品を読みます。 |
| `mypixiv users` | `pixiv mypixiv users [--page N --limit N --json\|--ndjson]` | 選択 account の MyPixiv user を一覧します。 |
| `mypixiv works` | `pixiv mypixiv works [USER_ID] --type illust\|manga\|novel [--page N --limit N --json\|--ndjson]` | MyPixiv works を一覧します。USER_ID 省略時は `illust` または `novel` だけです。 |
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
| `fanbox auth` | `pixiv fanbox auth import|list|use|remove|status` | local FANBOX session を import・管理します。session value は表示しません。native `--proxy`/`--no-proxy` は今回の FANBOX command だけに適用されます。 |
| `fanbox creators` | `pixiv fanbox creators [--kind supporting\|following] [--page N --limit N]` | supporting または following FANBOX creator を一覧します。 |
| `fanbox posts` | `pixiv fanbox posts SOURCE [--page N --limit N]` | creator、tag、post ID、対応 FANBOX URL の post を一覧します。 |
| `fanbox tags` | `pixiv fanbox tags CREATOR` | creator の featured tag を一覧します。 |
| `fanbox home` / `supporting` | `pixiv fanbox home|supporting [--page N --limit N]` | 認証済み FANBOX home または supporting feed を読みます。 |
| `fanbox post` | `pixiv fanbox post POST_ID` | 1 件の post と安全な asset summary を読みます。 |
| `fanbox download` | `pixiv fanbox download SOURCE...` | FANBOX post asset を設定済み download directory の下に保存します。 |
| `fanbox mcp` | `pixiv fanbox mcp [--proxy URL\|--no-proxy]` | read-only FANBOX MCP stdio server を起動します。native proxy override は FlareSolverr 設定を変更しません。 |

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
| `search` | `--draw-tool` | empty | この version の drawing-tool catalog にある正確な名前。唯一の 1 編集スペルミスには候補を示し、曖昧な prefix は拒否します。 |
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
| `download` | `--filename-template` | env/config/`{author} - {title}_{id}` | filename template。`{id}`、`{title}`、`{author}` だけを受け付けます。未知 placeholder または対応しない brace は error です。 |
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

`download` は interactive terminal の stderr に、すべての resource size を事前の HEAD probe で取得できた場合だけ batch byte progress を表示します。size が確定しない download と `--json`/`--ndjson` を含む非対話出力には progress text を混ぜません。安全に再開できる validated partial byte は開始時の進捗に含まれます。

### Drawing-tool catalog

`--draw-tool` は次の versioned catalog の exact value を受け取ります。unique な 1-edit spelling error は候補を返し、曖昧な prefix は error です。

```text
SAI
Photoshop
CLIP STUDIO PAINT
IllustStudio
ComicStudio
Pixia
AzPainter4
Painter
Illustrator
GIMP
FireAlpaca
網上描繪
AzPainter
CGillust
描繪聊天室
手畫博克
MS_Paint
PictBear
openCanvas
PaintShopPro
EDGE
drawr
COMICWORKS
AzDrawing
SketchBookPro
PhotoStudio
Paintgraphic
MediBang Paint
NekoPaint
Inkscape
ArtRage
AzDrawing4
Fireworks
ibisPaint
AfterEffects
mdiapp
GraphicsGale
Krita
kokuban.in
RETAS STUDIO
emote
4thPaint
ComiLabo
pixiv Sketch
Pixelmator
Procreate
Expression
PicturePublisher
Processing
Live2D
dotpict
Aseprite
Pastela
Poser
Metasequoia
Blender
Shade
3dsMax
DAZ Studio
ZBrush
Comi Po!
Maya
Lightwave3D
六角大王
Vue
SketchUp
CINEMA4D
XSI
CARRARA
Bryce
STRATA
Sculptris
modo
AnimationMaster
VistaPro
Sunny3D
3D-Coat
Paint 3D
VRoid Studio
筆芯筆
鉛筆
原子筆
毫筆
顏色鉛筆
Copic麥克筆
沾水筆
透明水彩
毛筆
記號筆
麥克筆
水溶性彩色铅笔
涂料
丙烯顏料
鋼筆
粉彩
噴筆
顏色墨水
蠟筆
油彩
COUPY-PENCIL
顏彩
```

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

認証済みの `detail`、pages、ugoira metadata は App API のみを使います。App のページ数不一致またはページ資源不足は明示的な typed failure を返します。認証済み ugoira は original ZIP が得られないとき、検証済み App medium ZIP を直接 download します。冪等 App JSON read だけは、最初の 429 に有効な `Retry-After` がある場合に command context 配下で一度だけ待機・再試行します。header 不正/欠落、二度目の 429、write、resource download は replay しません。
`detail --json` は Pixiv の raw HTML `caption` を保持し、通常の `detail` 出力は安全な plain text に変換します。作品 list output には caption を含めません。

`detail` は正の artwork ID、または HTTPS の `pixiv.net`/`www.pixiv.net` にある `/artworks/{id}` URL を受け付けます。locale、query、fragment は許可されます。user、novel、short-link、FANBOX、Pixivision、Sketch、legacy、そのほかの URL は受け付けません。

`download` は許可された CDN URL に加えて `/users/{id}`、`/users/{id}/artworks`、`/users/{id}/bookmarks/artworks`、`/user/{id}/series/{series_id}` を受け付けます。user/public-bookmarks/illustration-series source は `illust`、`manga`、`ugoira` を pagination で展開し、first-seen artwork ID で deduplicate します。series URL の owner は検証されます。`--filter EXPR` は artwork detail 後、file write 前に実行され、metadata を持たない CDN URL は拒否します。`--archive FILE` は SQLite archive で、選択 output と requested sidecar がすべて成功した artwork だけを記録します。`--write-metadata` は public artwork data と artifact-relative path を含む atomic `{artifact}.json` を書きます。template は `{id}`、`{title}`、`{author}`、`{author_id}`、`{date}`、`{tags}`、`{num}` をサポートし、directory template は safe relative path、`{num}` は zero-based です。resource download は valid `Retry-After` を持つ 429、5xx、retryable transport failure を既定で 3 回 retry（1/2/4 秒）し、validator が一致する partial だけを `If-Range` で安全に resume して atomic に publish します。`download` は action です。成功時 stdout は空で、安全な failure は stderr に出し、完了できない場合は non-zero で終了します。

### 共通 flag

| Flag | 適用先 | Default | 説明 |
| --- | --- | --- | --- |
| `--ndjson` | data list/read command | `false` | streaming filter/action 用に canonical Record を 1 行ずつ出力します。`--json` とは併用不可です。 |
| `--json` | safe data read、auth summary、`version`、`update --check` | `false` | command が対応する場合に complete result document を出力します。download/write action は success report を出しません。 |
| `--sleep-request DURATION` | network command と `mcp` | config/default | この invocation の request start 間隔。`PIXIV_REQUEST_INTERVAL` と `[network].request_interval` を上書きします。 |
| `--proxy URL` | network command と `mcp` | `https_proxy`/`HTTPS_PROXY`、`config.toml`、または empty | この command だけ `http`、`https`、`socks5`、`socks5h` proxy URI を使います。`auth import --file` では不可です。 |
| `--no-proxy` | `--proxy` と同じ | empty | この command の proxy を解除します。`--proxy` や bundle restore とは併用不可です。 |
| `--debug` | すべての CLI command、`mcp`、`fanbox mcp` | `false` | safe な real-time English diagnostics を stderr だけに書きます。log file を作らず、stdout、route、retry、result shape を変更しません。`auth export` と hidden OAuth callback は stderr を空のままにします。 |

### CLI が管理する `config` alias

`pixiv config get/set/unset` が受け付ける alias はこの table のものだけです。ほかの runtime setting は private `config.toml` を手動で管理します。CLI に generic setting editor はありません。

| KEY | Type | Default | 説明 |
| --- | --- | --- | --- |
| `account_pool_enabled` | boolean | `false` | safe な read/download 用の database account pool を有効化します。 |
| `account_pool_strategy` | string | `round_robin` | account pool の方式。`round_robin` または `random`。 |
| `download_path` | string | `./downloads` | download directory。 |
| `filename_template` | string | `{author} - {title}_{id}` | filename template。 |
| `directory_template` | string | empty | relative download directory template。 |
| `request_interval` | duration | `0` | network request start の minimum interval。`PIXIV_REQUEST_INTERVAL` と `--sleep-request` で上書きできます。 |
| `https_proxy` | string | empty | global `http`、`https`、`socks5`、`socks5h` proxy URI。lowercase `https_proxy` が優先。 |

手動 TOML には `[account_pool]`、`[network]`、`[pixiv.network]`、`[fanbox.network]`、`[fanbox.flaresolverr]`、`[login]`、`[update]` などを置けます：

```toml
[network]
https_proxy = "http://global-proxy.example:7890"

[pixiv.network]
proxy_url = "socks5h://pixiv-proxy.example:1080"

[fanbox.network]
proxy_url = ""                    # FANBOX native direct を明示
user_agent = "my-native-agent/1.0"

[fanbox.flaresolverr]
url = "http://127.0.0.1:8191"
proxy_url = "socks5://solver-upstream.example:1080"
```

`[pixiv.network].proxy_url` と `[fanbox.network].proxy_url` は absent と explicit empty を区別します。command `--proxy`/`--no-proxy` > 対応 service key（explicit empty を含む） > `https_proxy`/`HTTPS_PROXY` > `[network].https_proxy` > direct の順です。FANBOX native は userinfo のない HTTP(S) CONNECT のみ、Pixiv は HTTP(S)、SOCKS5、SOCKS5H を受け付けます。`user_agent` は FANBOX native header だけを変更し、Firefox 148 TLS profile を変更せず、Cloudflare 回避も保証しません。FlareSolverr は optional な challenge-only route で、service URL と upstream proxy は native FANBOX proxy と独立します。default config generator はこれらの optional table を作りません。

`[account_pool]` は `enabled` と `strategy` だけを保存し、各 account の `schedulable`、freeze、marker は `pixiv-cli.db` に保存します。legacy `account_pool.accounts` は database flag に一度だけ移行してから削除されます。`config.toml` に refresh token を書かないでください。歴史的な `data/account-pool.json` scheduler は自動的に読み取り・移行・削除されません。legacy の `[logging]` table は互換性のため無視され、`log_level` は `pixiv config` の対応 key ではありません。

v1 CLI は古い `~/.pixiv-cli/auth.json` を読み取らず、自動移行もしません。旧 version から切り替える前に、旧 CLI で
`pixiv auth export --all --output <private bundle>` を実行し、v1 で `pixiv auth import --file <bundle>` を実行してください。
移行は明示的に行い、古い file を暗黙の credential source にしません。

### 環境変数

| Variable | Default | 説明 |
| --- | --- | --- |
| `PIXIV_REFRESH_TOKEN` | empty | 対応する public SDK/MCP runtime の credential input。CLI data command は意図的に無視します。 |
| `DOWNLOAD_PATH` | `./downloads` | download directory。 |
| `FILENAME_TEMPLATE` | `{author} - {title}_{id}` | filename template。 |
| `DIRECTORY_TEMPLATE` | empty | relative download directory template。 |
| `PIXIV_REQUEST_INTERVAL` | empty | network request start の minimum interval。 |
| `https_proxy` / `HTTPS_PROXY` | empty | `http`、`https`、`socks5`、`socks5h` proxy URI。lowercase が優先。 |

CLI data command は pool 無効時は `pixiv auth use` の local default account、pool 有効時は database の eligible account を選びます。credential-selection flag と `PIXIV_REFRESH_TOKEN` は読みません。

設定優先順は service ごとです：command `--proxy`/`--no-proxy` > 対応 service proxy（explicit empty を含む） > `https_proxy`/`HTTPS_PROXY` > `[network].https_proxy` > direct。proxy override は保存されません。update は一般 network fallback だけを使い、FANBOX/solver 設定を消費しません。

### Debug diagnostics

`--debug` を command の前後に付けると、safe な lifecycle、account pool、network、challenge、solver、download、error event を確認できます：

```bash
pixiv --debug detail 123456
pixiv fanbox --debug post 12221352
pixiv --debug mcp 2>debug.log
```

各行は stderr にだけ書かれ、明確な product+subsystem module と完全な English sentence を持ちます。`logs/`、daily file、JSON event stream、raw URL、Cookie、token、signed query、proxy userinfo、clearance は出力しません。stdout と MCP JSON-RPC は変わりません。`pixiv auth export` は `--debug` があっても scope を作らず、raw-token/bundle stdout と空 stderr 契約を byte-for-byte 維持します。unknown option は scope 作成前に exit code `2` で報告されます。

### 削除された匿名 Web fallback

v1 は匿名 Web API fallback を削除しました。コンテンツコマンドは `pixiv auth use`
または手動 `[account_pool]` / `pixiv auth use` で選択した認証済みローカルアカウントを必要とし、
それがない場合は認証要求を返します。削除された `web_fallback_enabled` 設定が
`config.toml` に残っている場合は `removed_setting` を返し、
`pixiv config unset web_fallback_enabled` で消去できます。

無効なトークンと App API のネットワークまたはサーバーエラーは、安全で分類済みの
失敗を返します。

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
