# Pixiv SDK (v1)

[English](../en/sdk.md) | [简体中文](../zh-CN/sdk.md) | 日本語 | [ドキュメント索引](../index.md)

v1 SDK は 3 つの公開パッケージを提供します。

- `github.com/FlanChanXwO/pixiv-cli/sdk` — 両プロダクト共通のプロトコル非依存
  プリミティブ：ページング、不透明カーソル、分類済みエラー、リソース契約。
- `github.com/FlanChanXwO/pixiv-cli/sdk/pixiv` — Pixiv App API クライアント、
  モデル、URL 参照、mutation。
- `github.com/FlanChanXwO/pixiv-cli/sdk/fanbox` — Pixiv FANBOX クライアント、
  モデル、URL 解決。

すべての公開宣言は英語 GoDoc を持ちます。ソースコードが API の正規サマリーです。

## 認証

Pixiv は App-only です。すべてのコンテンツ操作は有効なアクセストークンを
必要とします。

```go
client, creds, err := pixiv.Open(ctx, refreshToken) // OAuth ローテーション
// コンテンツ要求の前に creds.RefreshToken() を永続化する

client, err := pixiv.New(accessToken) // 静的トークン、ネットワーク I/O なし
```

`Open` はアクセストークンだけを保持する Client を返し、自動 refresh はしません。
トークン失効後は操作が `CodeCredentialsExpired` を返します。匿名または Web
フォールバックは存在しません。

FANBOX は明示的な `FANBOXSESSID` 値で認証します：

```go
client, err := fanbox.Open(fanbox.SessionCredentials{FANBOXSESSID: session})
```

Pixiv リフレッシュトークンと FANBOX セッションは独立しており、変換されません。

## ページング

リスト操作は `sdk.Page[T]` と不透明な `Cursor` を返します。カーソルは product、
operation、binding version、クエリ要約に束縛され、別のクエリで再利用すると
`CodeInvalidCursor` を返します。

## エラー

すべての失敗は安定した `Code` を持つ `*sdk.Error` です：

```text
invalid_argument, invalid_cursor, unauthorized, credentials_expired, forbidden,
not_found, content_unavailable, challenge_required, rate_limited, upstream_error,
upstream_unavailable, malformed_upstream_response, resource_forbidden,
local_state_error, removed_setting
```

`errors.Is`/`errors.As` をサポートし、`context.Canceled`/`DeadlineExceeded` を
保持します。エラーチェーンに URL、ヘッダー、トークン、Cookie、設定内容は
含まれません。

## リソース

第一当事者メディアは `sdk.Resource` を通じて 2 つの並行パスで公開されます：

- `Resource.URL` + `Resource.RequestHeaders` — ディスクへバッファリングせず直接
  ストリームまたはプロキシ。
- `Resource.Ref` — `OpenResource`/`SaveResource` に渡して SDK 検証済み読み取り
  （scheme/host/path 再検証、Cookie なし、リダイレクト安全）。

`Resource` はトークンや Cookie を保持せず、`RequiresCredentials` は呼び出し元に
見えないプロダクト資格情報が必要な場合に真になります。

## URL 参照

`pixiv.ParseURL` と `fanbox.ResolveURL` はネットワークなしでページ URL を
型付き参照に変換し、`Reference.CanonicalURL` はトラッキングのない正規形を
返します。

## FANBOX

`sdk/fanbox` はクリエイタープロフィール、投稿、タグ、home / supporting
フィード、URL 解決、共有リソース契約を提供します。投稿本文は構造化ブロックで、
第三者の埋め込みは canonical リンクのみを保持します。制限投稿はサマリーのみで
Body は nil です。

## v0 からの移行

v0 `pixiv` から v1 `sdk/pixiv` への切り替えは[移行ガイド](../en/v1.0.0-migration.md)
を参照してください。
