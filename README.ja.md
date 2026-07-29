<div align="center">

# pixiv-cli

**Pixiv CLI · MCP stdio server · Go SDK**

[English](README.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md)

<p><a href="https://github.com/FlanChanXwO/pixiv-cli/actions/workflows/ci.yml"><img alt="Quality gate" src="https://github.com/FlanChanXwO/pixiv-cli/actions/workflows/ci.yml/badge.svg"></a> <a href="https://github.com/FlanChanXwO/pixiv-cli/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/FlanChanXwO/pixiv-cli?style=flat-square"></a> <a href="go.mod"><img alt="Go" src="https://img.shields.io/github/go-mod/go-version/FlanChanXwO/pixiv-cli?style=flat-square"></a> <a href="LICENSE"><img alt="License" src="https://img.shields.io/github/license/FlanChanXwO/pixiv-cli?style=flat-square"></a> <img alt="Views" src="https://hits.sh/github.com/FlanChanXwO/pixiv-cli.svg?style=flat-square&amp;label=views"></p>

[インストール](#インストール) · [クイックスタート](#60-秒クイックスタート) · [利用方法](#利用方法を選ぶ) · [ドキュメント](#ドキュメント) · [コントリビューション（English）](CONTRIBUTING.md)

</div>

`pixiv-cli` は Pixiv のエコシステムをターミナルにもたらします。作品とクリエイターを発見し、アカウントとブックマークを管理し、クリエイターをフォローして視覚作品をダウンロードできます。人・AI エージェント・Go アプリケーション向けの独立開発による非公式サードパーティーツールであり、Pixiv Inc. との提携・所属関係はなく、同社による承認も受けていません。CLI と MCP server は同じ public Go SDK を使用し、認証済み機能では Pixiv App API を信頼できるデータソースとします。利用時は Pixiv の規約および適用法令を遵守してください。

メンテナー向け：release tag は保護された認証 E2E gate により停止できます。refresh token は GitHub `pixiv-e2e` Environment Secret にのみ保存し、作品 ID と検索入力は Environment Variables に設定します。PR と `main` の CI は offline かつ secret-free のままです。詳細は[開発フロー](docs/maintainers/development.md#テスト)を参照してください。

## pixiv-cli を選ぶ理由

- **統一された機能** — CLI・MCP・SDK から公式 Pixiv の検索、詳細、ランキング、おすすめ、ユーザー、ブックマーク、フォロー、ダウンロード、うごイラを利用できます。第三者の集約/ランダム画像 API を再現しません。
- **App API 優先** — refresh token が設定されている場合は常に認証済み App 経路を使用し、App の失敗を Web に暗黙フォールバックしません。
- **認証済み R18 読み取り** — detail、pages、ugoira metadata と全 16 ranking mode は App API を使い、original が得られない場合は検証済み medium ugoira ZIP を正しく使用します。
- **実用的な検索フィルター** — レーティング、作品種別、AI モード、縦横比、解像度、動的な制作ツール候補に対応します。
- **Pixiv URL を直接指定** — 対応する作品 URL を detail/download に貼り付けられ、認証済みならユーザープロフィール/作品一覧 URL でそのユーザーの視覚作品をブラウザー Cookie 自動化なしにダウンロードできます。
- **ローカル複数アカウント OAuth** — ブラウザーの Cookie や profile を読み取らず、ブラウザーログイン、アカウント選択、refresh token rotation を行います。
- **安全な自動化** — typed SDK error、JSON 出力、クリーンな MCP stdio、署名付き更新を備え、結果を暗黙に切り捨てません。
- **限定的な匿名アクセス** — token がなく fallback が有効な場合、対応する読み取り操作は Web API を利用できます。

## インストール

### インストールスクリプト（Windows、Linux、macOS）

Linux/macOS（`sh`）：

```bash
curl -fsSLo /tmp/pixiv-install.sh https://github.com/FlanChanXwO/pixiv-cli/releases/latest/download/install.sh && sh /tmp/pixiv-install.sh --add-to-path
```

Windows Command Prompt（`cmd.exe`、PowerShell 不要）：

```bat
curl.exe -fsSLo "%TEMP%\pixiv-install.cmd" https://raw.githubusercontent.com/FlanChanXwO/pixiv-cli/main/scripts/install.cmd && call "%TEMP%\pixiv-install.cmd" --add-to-path
```

どちらのスクリプトも AMD64/ARM64 を検出し、最新の安定版公式 Release archive を選択して公開 SHA-256 を検証します。staging した binary を事前確認してからユーザー単位でインストールし、その後にのみ PATH を変更します。PATH を変更しない場合は `--no-path`、別の保存先には `--install-dir DIR` を使用できます。実行前にダウンロードしたスクリプトを確認できます。

正式版の installer は権威ある `checksums.txt` を常に GitHub HTTPS から直接取得します。内蔵の無料候補は platform archive の probe/download だけに使われ、返す checksum は直取得した内容と一致しなければなりません。download 後も SHA-256 を検証するため、これは転送到達性の改善であり、Release の identity や integrity 判定を変更しません。

### AI エージェントにインストールさせる

端末を操作できる Codex、Claude Code、Cursor などのローカル AI エージェントに、次の prompt をコピーしてください：

```text
Install the latest stable pixiv-cli from https://github.com/FlanChanXwO/pixiv-cli for this machine: inspect the repository's scripts/install.sh or scripts/install.cmd first, choose the script matching the detected OS and architecture (the Windows path must use cmd.exe and must not invoke PowerShell), download only official GitHub Release assets, require the published SHA-256 check to pass before replacing anything, install per-user without administrator or root privileges, add only the chosen install directory to the user PATH, ask before installing any missing prerequisite, never read or output Pixiv credentials, verify with pixiv version, and report the installed version plus every file and PATH change.

Also install the product skill that matches the same stable release tag (not main): download the full skills/pixiv-cli/ directory from that tag into the agent skills directory the user confirms. Do not guess the skills path and do not follow the main branch for skill content.
```

### SkillHub から product Skill をインストールする

SkillHub に対応した Agent は、公開済みの [`pixiv-cli` Skill](https://www.skillhub.cn/skills/pixiv-cli) を SkillHub から直接インストールできます。Skill には独自の version があり、インストール済み CLI の使い方を案内します。command の syntax は常に `pixiv <cmd> --help` を最終的な根拠にしてください。

### ClawHub から product Skill をインストールする

ClawHub を使う Agent は、公開済みの [`pixiv-cli` Skill](https://clawhub.ai/flanchanxwo/skills/pixiv-cli) を `clawhub install pixiv-cli` でインストールできます。unversioned latest を追わず、CLI release と同じ公開 Skill version に固定してください。

### Homebrew（macOS/Linux の推奨方法）

```bash
brew install FlanChanXwO/tap/pixiv-cli
```

更新時：

```bash
brew update
brew upgrade pixiv-cli
```

### Go

公開済みの正確な tag を指定してください。source install には Go、cgo、C linker、および対象 platform 用に commit された Rust static library が必要です。

```bash
go install github.com/FlanChanXwO/pixiv-cli/cmd/pixiv@vX.Y.Z
```

### Release archive または source build

[GitHub Releases](https://github.com/FlanChanXwO/pixiv-cli/releases) から対応 archive を取得するか、checkout を build します：

```bash
sh scripts/build.sh
```

直接配布には checksum と署名付き manifest が含まれます。platform と信頼境界の詳細は [CLI リファレンス](docs/ja/cli-reference.md#インストール)を参照してください。

## 60 秒クイックスタート

```bash
# ブラウザー OAuth で Pixiv アカウントを保存します。
pixiv auth login

# App 側フィルターを使って検索します。
pixiv search "初音ミク" --type illust --ai-mode exclude --resolution high
pixiv novel search "初音ミク" --rating sfw --min-text-length 1000

# クリエイターをフォローし、作品をブックマークします。
pixiv follow add 12345678
pixiv bookmark add 123456

# 詳細、おすすめ、ダウンロードを利用します。
pixiv detail https://www.pixiv.net/artworks/123456
pixiv recommended all --limit 10
pixiv download https://www.pixiv.net/artworks/123456 --pages 1,3-5 --quality regular
pixiv download 123456 https://i.pximg.net/img-original/example.jpg --concurrency 8

# クリエイターの全視覚作品を一括でダウンロードします。
pixiv download https://www.pixiv.net/users/12345678/artworks
```

すべての command、flag、設定キー、環境変数、fallback、更新動作は `pixiv --help` または[完全な CLI リファレンス](docs/ja/cli-reference.md)で確認できます。

## 利用方法を選ぶ

### CLI

対話操作では読みやすい既定出力を使い、機械処理では対応 command に `--json` を指定します：

```bash
pixiv ranking --mode day --json
pixiv user search "miku" --limit 10 --json
pixiv user detail 12345678
pixiv search-options "初音ミク"
```

### MCP

stdio server は明示的に起動します。stdout は JSON-RPC 専用です。操作要約は `~/.pixiv-cli/logs`（Windows では `%USERPROFILE%\.pixiv-cli\logs`）の日次 plain-text file `YYYY-MM-DD.txt`（既定保持 7 日）に書き、端末は既定で log 痕跡を出しません。

```bash
pixiv mcp
```

tool、parameter、structured output、認証動作は [MCP tool contract](docs/ja/mcp-tools.md)を参照してください。
MCP の固定 status、error、display text は英語です。Pixiv metadata と user-supplied text は原文のまま保持します。

### Go SDK

```go
client, err := pixiv.OpenDefault()
if err != nil {
    // ローカルの認証・設定エラーを処理します。
}
result, err := client.SearchIllust(ctx, pixiv.SearchIllustRequest{Word: "初音ミク"})
download, err := client.Download(ctx, "https://www.pixiv.net/artworks/123456")
```

`github.com/FlanChanXwO/pixiv-cli/pixiv` を import します。`Download`/`DownloadAll` は文書化された初心者 default を使い、`DownloadWith`/`DownloadAllWith` は path、naming、page、quality、concurrency を制御します。モデル、cursor、resource、error、呼び出し側の責務は [SDK ガイド](docs/ja/sdk.md)を参照してください。

## 認証と token の安全性

推奨設定は `pixiv auth login` です。Pixiv App OAuth の raw refresh token を UID ごとにローカル account store へ保存します。`PHPSESSID` などのブラウザー Cookie は拒否され、App credential へ変換されません。

macOS、desktop Linux、Windows は on-demand persistent `pixiv://` callback handler を使い、local login と明示設定した cross-machine relay をサポートします。relay に送れるのは `pixiv://account/login` だけで、Cookie 読み取り、browser automation、Web fallback は行いません。GUI のない SSH server は既存の `--no-open --addr` と local `ssh -L` tunnel を使うか、documented relay server/client setting を設定します。詳細は [CLI リファレンス](docs/ja/cli-reference.md#refresh-token-の取得) を参照してください。

```bash
pixiv auth list
pixiv auth use 12345678
pixiv auth check
```

import は引数なしの非表示入力または raw stdin を推奨します。位置引数の token は argv/shell history に残ります。stdout へ secret を出力できるのは、`--output` を付けない `pixiv auth export [UID]` と `pixiv auth export --all` だけです。ファイルには `--output` で private bundle を作成します。bundle は暗号化されていない point-in-time backup であり live sync ではなく、token rotation 後は stale になる場合があります。secret 出力を chat、log、shell history、issue、Agent transcript に貼り付けないでください。他の stdout/stderr、JSON、MCP result、log、error は refresh token を公開してはなりません。完全な契約は [CLI リファレンス](docs/ja/cli-reference.md#refresh-token-の取得)を参照してください。

## ドキュメント

| ガイド | 内容 |
| --- | --- |
| [CLI リファレンス](docs/ja/cli-reference.md) | command、flag、認証、設定、fallback、download、update |
| [Go SDK](docs/ja/sdk.md) | public client、model、pagination、resource、typed error |
| [MCP tools](docs/ja/mcp-tools.md) | tool schema と出力 semantics |
| [アーキテクチャ（中国語・簡体字）](docs/maintainers/architecture.md) | package boundary と runtime flow |
| [開発フロー（中国語・簡体字）](docs/maintainers/development.md) | toolchain、test、build、release |
| [Changelog](changelog/README.md) | ユーザーに影響する変更 |

## コントリビューション

bug report、文書修正、test、範囲を絞った機能追加を歓迎します。pull request の前に [CONTRIBUTING.md](CONTRIBUTING.md) を読み、大規模または public interface に影響する変更は先に相談してください。

## ライセンス

[MIT](LICENSE)
