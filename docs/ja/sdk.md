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
トークン失効後は操作が `CredentialsExpired` を返します。匿名または Web
フォールバックは存在しません。OAuth の成功レスポンスには正の account user ID が必要で、
identity がない場合は Client や credentials を返さず `MalformedUpstreamResponse` になります。

### プログラムからのブラウザーログイン

`BeginLogin` は self-contained な one-shot PKCE session を作成します。ブラウザーを
開いたり loopback listener を起動したりしません。呼び出し側が `AuthorizationURL()`
を開き、callback URL または bare code を `Complete` に渡します。

```go
session, err := pixiv.BeginLogin(pixiv.LoginOptions{HTTPClient: httpClient})
if err != nil { /* エラー処理 */ }
if !session.AcceptsCallbackURL(callbackURL) {
    // one-shot session を消費する前に callback を拒否する。
}
credentials, err := session.Complete(ctx, callbackURL)
```

`AcceptsCallbackURL` は非消費的で network I/O を行いません。公式 HTTPS callback は
session の `state` を必須とし、対応する `pixiv://account/login` callback は `state` を
省略できますが、指定した値は一致しなければなりません。`IsOfficialOAuthCallbackURL` と
`IsOfficialOAuthStartURL` は exact origin/path を network なしで検証します。session の
format 出力は verifier、state、authorization code、callback URL を露出しません。

FANBOX は明示的な `FANBOXSESSID` 値で認証します：

```go
client, err := fanbox.Open(fanbox.SessionCredentials{FANBOXSESSID: session})
```

Pixiv リフレッシュトークンと FANBOX セッションは独立しており、変換されません。

FANBOX の接続 option は明示的な任意設定です：

```go
client, err := fanbox.OpenWith(credentials, fanbox.Options{
    ProxyURL:  "https://proxy.example:8443", // native HTTP(S) CONNECT のみ
    UserAgent: "my-native-agent/1.0",          // native header のみ
    FlareSolverr: &fanbox.FlareSolverrOptions{
        URL:      "http://127.0.0.1:8191",
        ProxyURL: "socks5://solver-upstream.example:1080",
    },
})
```

空の `UserAgent` は組み込み Firefox 148 baseline を使います。custom 値は TLS profile を変更せず、
Cloudflare 回避を保証しません。`FlareSolverr` が nil なら完全に無効で、native request が厳密に
Cloudflare challenge と判定された場合だけ呼び出されます。solver service URL と upstream proxy は
native proxy と独立しており、public constructor は network I/O を行いません。

## ページング

リスト操作は `sdk.Page[T]` と不透明な `Cursor` を返します。カーソルは product、
operation、binding version、クエリ要約に束縛され、別のクエリで再利用すると
`InvalidCursor` を返します。

## エラー

すべての失敗は安定した `Reason` を持つ `*sdk.Error` です：

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
  （scheme/host/path 再検証とリダイレクト安全）。`Resource` 自体は Cookie を保持せず、束縛された
  FANBOX client も FANBOX API と `downloads.fanbox.cc` の policy に限って session を使い、Pixiv/CDN
  や第三者 host には送信しません。

`Resource` はトークンや Cookie を保持せず、`RequiresCredentials` は呼び出し元に
見えないプロダクト資格情報が必要な場合に真になります。

## URL 参照

`pixiv.ParseURL` と `fanbox.ResolveURL` はネットワークなしでページ URL を
型付き参照に変換し、`Reference.CanonicalURL` はトラッキングのない正規形を
返します。

## FANBOX

`sdk/fanbox` はクリエイタープロフィール、投稿、タグ、home / supporting
フィード、URL 解決、共有リソース契約を提供します。検証済み native route は
`api.fanbox.cc` root の `post.info`、`post.listHome`、`post.listSupporting`、
`post.listTagged`、`tag.getFeatured` を使い、creator pagination は server の `pageUrls`
に従います。投稿本文は構造化ブロックで、画像・ファイルブロックはリソース index と結び付けます。
上流が `imageMap` または `fileMap` だけで添付を提供する場合も、利用可能なリソースを公開します。
第三者の埋め込みは canonical リンクのみを保持します。制限投稿はサマリーのみで Body は nil です。

## v0 からの移行

v0 `pixiv` から v1 `sdk/pixiv` への切り替えは[移行ガイド](../en/v1.0.0-migration.md)
を参照してください。
