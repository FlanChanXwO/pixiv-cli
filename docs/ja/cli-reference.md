# Pixiv CLI リファレンス

[English](../en/cli-reference.md) | [简体中文](../zh-CN/cli-reference.md) | 日本語 | [プロジェクトホーム](../../README.ja.md)

これは `pixiv` command の完全な契約です。インストール、認証、command、flag、設定、環境変数、匿名
fallback、更新を扱います。SDK と MCP の詳細は重複させず、[関連ドキュメント](#関連ドキュメント)へ
案内します。

ユーザーに影響する変更は [CHANGELOG.md](../../CHANGELOG.md) に記録されます。

## インストール

> **Release の状態**：対応 binary の Ed25519 public key、key ID、fingerprint は
> [`internal/bootstrap/release_trust.go`](../../internal/bootstrap/release_trust.go) に commit されています。
> public source/tap repository、保護された `release` Environment、分離された credential は設定済みです。
> v0.3.0 は 6 platform archive、checksum、署名付き manifest を含む公式 GitHub Release として公開され、
> stable Homebrew formula も push 済みです。将来の version も同じ tag、署名、asset、Homebrew gate を
> 通過するまでは信頼済み download source とみなしません。

### 公式インストールスクリプト

最新 stable Release をユーザー単位で導入する 2 種類の bootstrap script を提供します：

```bash
# Linux/macOS
curl -fsSLo /tmp/pixiv-install.sh https://raw.githubusercontent.com/FlanChanXwO/pixiv-cli/main/scripts/install.sh
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

v0.3.0 より後に build される Linux Release asset は glibc 2.35 以降を必要とします。release、
native-evidence、packaged-smoke job は両方の Linux architecture を Ubuntu 22.04 上で build し、GNU
version requirement が `GLIBC_2.35` を超える ELF を拒否します。v0.3.0 はこの gate より前の asset
で `GLIBC_2.39` を要求する場合があり、Debian 12 とは互換性がありません。installer の binary
preflight は既存の install を置き換える前に loader failure を明示します。

初回 bootstrap の script には Ed25519 verifier を埋め込めません。SHA-256 は破損・取り違えを検出しますが、
真正性は HTTPS と公式 GitHub repository/Release account に依存します。実行前に script を確認してください。
導入後の `pixiv update` は binary に埋め込まれた Ed25519 trust root を使用します。

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

### 直接ダウンロード

Release は darwin、linux、windows の amd64/arm64 向けに
`pixiv-cli_<version>_<os>_<arch>.tar.gz`（Windows は `.zip`）を生成し、`checksums.txt` と Ed25519 署名付き
`checksums.json` を添付します。v0.3.0 は 6 種類すべてを含みます。

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
| Browser | macOS では一時的な `pixiv://` callback helper を登録して default browser を開き、既存 Pixiv session を再利用できます。`--no-open` では login URL と local page だけを表示します。 |
| Callback | この試行の loopback callback、一時 helper からの hand-off、terminal paste、local page form だけを受理します。戻らない場合は callback URL、`pixiv://...` URL、Pixiv relay URL、raw authorization code を貼り付けられます。 |
| 検証 | local callback はこの試行の state と一致する必要があります。Pixiv が state を返さない場合だけ、公式 callback URL と `pixiv://account/login` を明示 fallback として使えます。 |
| 保存 | refresh/access token は表示せず、refresh token を UID ごとに `auth.json` へ保存します。Unix-like は parent `0700`・file `0600`、Windows は既存 ACL を維持します。 |

macOS の `PixivCLIURLHandler.app` は現在の login 試行だけを転送し、browser cookie、storage、history、session
file、tab、network traffic を読みません。利用できなくても通常の browser と loopback/manual paste を使い、
managed Chromium、DevTools/CDP、browser state scan は行いません。relay URL はこの OAuth 試行に属すると
確認した後、一度だけ開きます。browser が空白ページでも terminal の最終結果を正とし、callback がなければ
成功を偽装しません。

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
だけに接続し、callback port を公開しません。別案として interactive SSH terminal を使い、最終 callback
URL、`pixiv://` URL、relay URL、raw authorization code を元の `auth login` prompt に貼り付けられます。
login listener を public interface に bind しないでください。`--addr` は意図的に loopback address だけを
受理します。

browser の system proxy は Go CLI に自動継承されません。必要なら先に設定します：

```bash
pixiv config set https_proxy http://127.0.0.1:7890
pixiv auth login --proxy http://127.0.0.1:7890
```

`--proxy URL` と `--no-proxy` は現在の command だけに適用され、同時使用できず、`config.toml` に保存されません。
実 Pixiv login は OAuth Web flow に依存し、自動 test は fake OAuth server を使用します。

### 認証の import

v0.4.0 では `auth add`、`auth token`、`--token` を alias なしで削除します。direct import は次の形式です：

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

credential は Pixiv UID ごとに `os.UserConfigDir()/pixiv/auth.json`、global setting は
`os.UserConfigDir()/pixiv/config.toml` に保存されます。既定は読みやすい出力で、対応 command は `--json` を
利用できます。`auth export` は意図的に JSON flag を持ちません。Cobra/pflag の option は positional argument
の前後どちらでも指定できます。

### CLI command 一覧

| Command | Usage | 説明 |
| --- | --- | --- |
| `auth import` | `pixiv auth import [REFRESH_TOKEN] [--file PATH] [--json] [--proxy URL\|--no-proxy]` | direct input は rotation 後の token を検証・保存します。引数なし TTY は非表示、非 TTY は raw stdin です。`--file PATH|-` は offline atomic bundle restore となり、token/proxy input と競合します。 |
| `auth login` | `pixiv auth login [--json] [--no-open] [--addr 127.0.0.1:0] [--use] [--timeout DURATION] [--proxy URL\|--no-proxy]` | loopback server と browser OAuth で login し、token を表示せず UID ごとに保存します。 |
| `auth list` | `pixiv auth list [--json]` | local account を一覧表示し、refresh token は表示しません。 |
| `auth export` | `pixiv auth export [UID] [--all] [--output PATH] [--force]` | default/指定/all account を local export します。`--output` なしは single raw token または `--all` bundle、指定時は private bundle と安全な stdout summary です。 |
| `auth use` | `pixiv auth use [UID] [--json]` | default account を設定し、TTY では対話選択できます。 |
| `auth remove` | `pixiv auth remove [UID] [--yes] [--json]` | account を削除します。default 削除後は残りの先頭を選択します。 |
| `auth check` | `pixiv auth check [UID] [--json] [--proxy URL\|--no-proxy]` | token を refresh して account を検証します。 |
| `config path` | `pixiv config path` | `config.toml` path を表示します。 |
| `config get` | `pixiv config get KEY` | 有効な設定値を 1 件表示します。 |
| `config set` | `pixiv config set KEY VALUE` | 既知の設定キーを書き込みます。 |
| `config unset` | `pixiv config unset KEY` | 既知の設定キーを削除します。 |
| `version` | `pixiv version [--json]` | `version`、`commit`、`build_date` を表示します。root `pixiv --version` は version だけです。 |
| `update` | `pixiv update [--check] [--prerelease] [--proxy URL]` | 現在の install source に合わせて確認・更新します。`--json` は `--check` とだけ使用できます。 |
| `search` | `pixiv search [options] WORD` | イラストを検索します。 |
| `search-options` | `pixiv search-options [options] WORD` | keyword に対する App API 制作ツール候補を表示します。認証が必要です。 |
| `detail` | `pixiv detail [options] ILLUST_ID` | 作品詳細を表示します。 |
| `ranking` | `pixiv ranking [options]` | イラストランキングを表示します。 |
| `recommended` | `pixiv recommended all\|illust\|manga\|novel\|user [--page N --limit N --json]` | 指定 kind の認証済みおすすめを表示します。`all` は 4 種類を順に返します。 |
| `user detail` | `pixiv user detail USER_ID [--json]` | ユーザーの完全な公開 profile を表示します。 |
| `user artworks` | `pixiv user artworks [USER_ID] [--type TYPE --page N --limit N]` | 作品一覧。UID 省略時は現在の認証ユーザーです。 |
| `user bookmarks` | `pixiv user bookmarks [USER_ID] [--restrict public\|private --tag TAG --page N --limit N]` | visibility/tag で絞った bookmark 一覧。 |
| `user following` | `pixiv user following [USER_ID] [--restrict public\|private --page N --limit N]` | follow 一覧。 |
| `bookmark add` | `pixiv bookmark add ILLUST_ID [--restrict public\|private --tag TAG...]` | bookmark を追加します。`--tag` は反復可能です。 |
| `bookmark remove` | `pixiv bookmark remove ILLUST_ID` | bookmark を削除します。visibility/tag flag はありません。 |
| `follow add` | `pixiv follow add USER_ID [--restrict public\|private]` | 指定 visibility で follow します。 |
| `follow remove` | `pixiv follow remove USER_ID` | unfollow します。visibility flag はありません。 |
| `download` | `pixiv download [options] ILLUST_ID...` | 1 件以上の作品を download します。token なしでは既定で匿名 Web fallback を使います。 |
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
| `--proxy URL` / `--no-proxy` | empty | この token exchange だけの proxy override で、保存されません。 |

### Data command flag

| Command | Flag | Default | 説明 |
| --- | --- | --- | --- |
| `search` | `--search-target` | `partial_match_for_tags` | 検索範囲。 |
| `search` | `--sort` | `date_desc` | sort order。 |
| `search` | `--duration` | empty | Pixiv API duration。 |
| `search` | `--rating` | `all` | `sfw`, `r18`, `r18g`, `mature`, `all`。 |
| `search` | `--type` | `all` | `all`, `illust-and-ugoira`, `illust`, `manga`, `ugoira`。`comics` は `manga` の互換 alias。 |
| `search` | `--ai-mode` | `all` | `all`, `exclude`, `only`。Pixiv `AIType==2` が AI 生成です。 |
| `search` | `--ai-type` | `2` | deprecated alias：`0=exclude`, `1=only`, `2=all`。明示 `--ai-mode` と競合します。 |
| `search` | `--aspect-ratio` | `all` | `all`, `landscape`, `portrait`, `square`。 |
| `search` | `--resolution` | `all` | `all`, `high`, `medium`, `low`。両辺がそれぞれ `>=3000`, `1000..2999`, `<=999`。 |
| `search` | `--tool` | empty | upstream の正確な制作ツール名。認証済み `search-options` で取得します。 |
| list commands | `--limit` | one upstream batch | 最大件数。`0` は next batch がなくなるまで取得します。 |
| list commands | `--page` | empty | 1-based logical page。正数 `--limit` が必要です。 |
| list commands | `--offset` | `0` | deprecated logical offset。`--page` と併用不可。 |
| `search` | `--r18` | `false` | deprecated `--rating r18` alias。明示 non-R18 rating と競合します。 |
| `ranking` | `--mode` | `day` | ranking mode。 |
| `ranking` | `--date` | empty | 通常 `YYYY-MM-DD`。 |
| `ranking` | `--offset` | `0` | pagination offset。 |
| `recommended KIND` | `--page`, `--limit`, deprecated `--offset` | per-stream | `all` でも 4 stream を独立に pagination します。 |
| `user artworks` | `--type` | `illust` | user-artworks request の種類。 |
| `user bookmarks` | `--restrict` | `public` | `public` または `private`。 |
| `user bookmarks` | `--tag` | empty | 正確な bookmark tag filter。 |
| `user following` | `--restrict` | `public` | follow visibility。 |
| `bookmark add` | `--restrict` | `public` | 新規 bookmark visibility。 |
| `bookmark add` | `--tag` | empty | 反復可能な bookmark tag。 |
| `follow add` | `--restrict` | `public` | 新規 follow visibility。 |
| `detail` | `ILLUST_ID` | required | Pixiv artwork ID。 |
| `download` | `ILLUST_ID...` | required | 1 件以上の artwork ID。 |

refresh token がある `search` は常に App API を使います。App は解像度、縦横比、tool、content type、
`ai-mode=exclude` を処理し、rating と `ai-mode=only` は App result batch ごとに適用されます。App failure は
Web に fallback しません。filter は opaque cursor に binding され、別条件へ再利用できません。正数
`--limit`/`--page` では必要件数、upstream 終端、repeated cursor まで batch を取得し、`--limit` なしでは
互換の 1 batch 既定を維持します。bookmark-count filter はありません。

### 共通 flag

| Flag | 適用先 | Default | 説明 |
| --- | --- | --- | --- |
| `--uid UID` | `search/search-options/detail/ranking/recommended/user/download` | `auth.json.default_user_id` | local account を選びます。 |
| `--profile UID` | 同上 | empty | deprecated `--uid` alias。 |
| `--refresh-token TOKEN` | 同上 | empty | account/env token を一時上書きします。raw App token だけを受け付けます。 |
| `--json` | `auth import/login/list/use/remove/check`、`version`、`update --check`、data commands | `false` | machine-readable JSON。`auth export` と実更新にはありません。 |
| `--download-path PATH` | data commands（実質 `download`） | env/config/`./downloads` | download directory。 |
| `--filename-template TEMPLATE` | data commands（実質 `download`） | env/config/`{author} - {title}_{id}` | filename template。 |
| `--proxy URL` | direct-token `auth import`、`auth login/check`、data/`mcp` | env/config/empty | この command だけの HTTP(S) proxy。`auth import --file` では使用不可。 |
| `--no-proxy` | 同上 | empty | この command の proxy を解除します。`--proxy` または bundle restore と併用不可。 |

### 対応 `config` key

| KEY | Type | Default | 説明 |
| --- | --- | --- | --- |
| `download_path` | string | `./downloads` | download directory。 |
| `filename_template` | string | `{author} - {title}_{id}` | filename template。 |
| `https_proxy` | string | empty | HTTP(S) proxy。lowercase env が優先。 |
| `web_fallback_enabled` | bool | `true` | token がない場合に匿名 Web fallback を許可します。 |
| `log_level` | string | `warn` | stderr log level。`PIXIV_LOG_LEVEL` で上書き。 |
| `log_format` | string | `text` | `text` または `json`。 |
| `update_check_enabled` | bool | `true` | 通常 command 成功後の stable update check。 |
| `output_json` | bool | `false` | data command の既定を JSON にします。 |
| `login_open_browser` | bool | `true` | `auth login` で browser を自動起動します。 |
| `login_timeout` | duration | `0s` | login の既定待機時間。 |
| `login_use_after_login` | bool | `false` | login 後に default account にします。 |

### 環境変数

| Variable | Default | 説明 |
| --- | --- | --- |
| `PIXIV_REFRESH_TOKEN` | empty | App API OAuth refresh token。account/flag で上書き可能。 |
| `PIXIV_LOG_LEVEL` | empty | `log_level` を上書き。 |
| `PIXIV_LOG_FORMAT` | empty | `log_format` を上書き。 |
| `DOWNLOAD_PATH` | `./downloads` | download directory。 |
| `FILENAME_TEMPLATE` | `{author} - {title}_{id}` | filename template。 |
| `https_proxy` / `HTTPS_PROXY` | empty | HTTP(S) proxy。lowercase が優先。 |

認証優先順：`--refresh-token` > `--uid`/deprecated `--profile` > `PIXIV_REFRESH_TOKEN` >
`auth.json.default_user_id`。

設定優先順：CLI flag > environment > `config.toml` > built-in default。proxy override は保存されません。

### 匿名 Web fallback

token source がなく `web_fallback_enabled=true` の場合、CLI の `search`、`detail`、`ranking`、`download`
は Pixiv Web/ajax API を利用できます。refresh token がある場合は App API を優先し、invalid token、network、
server error を自動 fallback しません。

- 匿名 `search` は Web が確実に表現できる filter だけを使用します。AI は返却 field で判定します。
- `rating=r18|r18g|mature` は request 前に認証要求として失敗し、空結果に見せません。`all` は匿名で見える範囲です。
- `search-options` は App 専用です。Cookie を読み取らず、refresh token を Web session に変換しません。
- `search_user` は公式 user search ではなく、work search の author を `userId` で dedupe します。
- 静止画は `/ajax/illust/{id}/pages` の `original`、ugoira は `/ajax/illust/{id}/ugoira_meta` の
  `originalSrc` と frame を使い、対応 build は内蔵 Rust encoder で GIF/APNG を生成します。
- 専用 proxy env はなく、共通 `--proxy`、environment、config を使います。

invalid proxy URL は network 前に失敗し、diagnostic に userinfo、path、query を出しません。無効化：

```bash
pixiv config set web_fallback_enabled false
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

update check は canonical SemVer tag だけを選びます。対象 channel に non-SemVer published Release があれば、
古い version へ暗黙に戻らず fail closed します。private key は保護された `release` Environment と管理された
recovery copy にだけ存在します。

通常 command 成功後の best-effort stable check は MCP、help、`version`、`update`、全 `auth export`、`auth import --file`、development
build を除外し、user cache ごとに 24 時間 1 回、3 秒上限です。新 version や check failure は stderr のみに
出し、business exit code、JSON stdout、MCP stdout を汚しません。無効化：

```bash
pixiv config set update_check_enabled false
```

## 関連ドキュメント

- [Go SDK（English）](../en/sdk.md)：public client、model、pagination、resource、typed error。
- [MCP tools（English）](../en/mcp-tools.md)：tool、input schema、output、stdio behavior。
- [アーキテクチャ（中国語・簡体字）](../maintainers/architecture.md)：package responsibility と runtime flow。
- [開発フロー（中国語・簡体字）](../maintainers/development.md)：environment、test、build、release gate。
- [Agent skill](../../skills/pixiv-cli/SKILL.md)：インストール済み CLI を安全に操作するための指示。
