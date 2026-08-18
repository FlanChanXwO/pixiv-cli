# Authentication import and export

Use these workflows only when the user explicitly asks to import, export, back
up, or restore authentication. Refresh tokens and every export bundle are
plaintext secrets.

## Browser OAuth login

Run `pixiv auth login` only on the user's explicit request while they are
present to approve the Pixiv page. On macOS, desktop Linux, and Windows, the
CLI uses a persistent, on-demand current-user `pixiv://` handler. Its active
loopback bridge has priority; otherwise only `pixiv://account/login` can reach
a current one-time remote hand-off, while other `pixiv://` URLs are delegated to
the previous handler. Do not invoke hidden protocol-handler helpers directly.

For a cross-machine login, configure `login_relay_public_url` and
`login_relay_listen_addr` on the account-hosting server. `pixiv auth login`
prints a one-login hand-off URL rather than an authorization URL. Opening it
redirects directly to `pixiv://account/remote-login`; the local installed CLI
claims that one session, starts OAuth, and returns the callback to the
account-hosting server. The local handoff state is valid for that session only,
and a new handoff replaces the previous local handoff.

A remote login requires that installed desktop handler. pixiv-cli does not
render a project confirmation page or accept a manually copied callback for this
flow.

The relay can use HTTP or HTTPS. `login_relay_secret` and
`login_relay_target_url` are silently ignored. `pixiv auth devices` has been removed.
`pixiv config` manages account-pool enabled/strategy, download path, filename
template, and HTTPS proxy;
advanced relay settings belong in the private `config.toml`.

## Account-pool scheduling

The account pool applies only to safe, non-mutating Pixiv reads and downloads.
Set `account_pool_enabled=true` and optionally `account_pool_strategy=random`
through `pixiv config`, then use the database-backed commands below to manage
which local Pixiv accounts participate:

```text
pixiv auth pool status --json
pixiv auth pool enable UID... [--all]
pixiv auth pool disable UID... [--all]
```

`auth pool status` and `auth list` expose only non-secret scheduling summaries.
The removed `account_pool.accounts` config key is not migrated; if it remains,
runtime configuration fails with `removed_setting`. Remove it explicitly with
`pixiv config unset account_pool_accounts`. The historical
`data/account-pool.json` scheduler is not read, migrated, or deleted by the
current runtime.

## Import one refresh token

Choose the input path according to where the secret currently exists:

- If the user already put the refresh token in the conversation and explicitly
  asks the agent to import it, disclose that execution copies it into the tool
  call, command argument, shell history, and process context. An already
  disclosed token cannot be erased. After that disclosure, run the positional
  form `pixiv auth import 'RFT'` with the actual value safely shell-quoted. Do
  not restate the token, create a token file, or include it in the result.
- If the token has not been disclosed, do not ask for it in chat. Start
  `pixiv auth import` for its hidden prompt only when the runtime provides an
  interactive terminal that the user can type into directly. A standard agent
  PTY may have no user-input channel; do not launch the command there and leave
  it waiting. Give `pixiv auth import` to the user for their private terminal,
  or pipe an authorized real secret-manager retrieval command directly to it
  in one shell command. With non-TTY stdin the CLI reads the token
  automatically. Do not invent or pass a `--stdin` flag. This routing reflects
  the environment's interaction capability, not a ban on agent execution.

`--proxy URL` and `--no-proxy` apply only to the OAuth validation performed by
a single-token import; they are mutually exclusive. Both successful text output
and `--json` output are safe to report because they omit the token, but always
check the exit status before reading either output.

## Restore an export bundle

```
pixiv auth import < /private/path/pixiv-auth.json
pixiv auth export --all | ssh trusted-host pixiv auth import
```

The v1 CLI never reads or migrates the old `~/.pixiv-cli/auth.json`. When
moving from an older CLI, run `pixiv auth export --all --output <private
bundle>` with the old version, then restore that private bundle through shell
redirection or a pipe such as `pixiv auth import < bundle.json`.

With no positional token, the import classifier checks the first non-whitespace
byte of non-TTY stdin. `{` selects strict versioned bundle decoding; any other
input is one opaque refresh token. Bundle decoding is completely offline and a
failure never falls back to OAuth. An explicit positional value is always an
opaque token, even when it starts with `{`. Bundle input cannot be combined
with `--proxy` or `--no-proxy`; those flags apply only to single-token OAuth
validation. The bundle itself contains plaintext refresh tokens, even though
normal text/JSON success output and errors do not echo them. Do not inspect,
summarize, or log bundle content.

## Export safely

The output mode changes what stdout contains:

- `pixiv auth export [UID]` writes that account's raw refresh token to stdout.
- `pixiv auth export --all` writes a versioned all-account bundle to stdout.
- `--output PATH` writes a private versioned bundle instead of putting a secret
  on stdout. It refuses to replace an existing path unless `--force` is also
  supplied. Use `--force` only when the user explicitly intends replacement.

Run a bare stdout export only when the user explicitly asks to receive or see
the raw token or bundle for that invocation. Before executing, explain that the
secret necessarily enters tool output/transcript and may be retained there,
then obtain the user's explicit confirmation. The binary's raw stdout is the
only permitted disclosure: after execution, do not restate, quote, reformat,
parse, summarize, or log it. In every other case, use `--output` or connect
stdout directly to its intended consumer in the same shell command, for
example:

```
pixiv auth export UID | consumer-command
pixiv auth export --all | consumer-command
pixiv auth export --all --output /private/path/pixiv-auth.json
```

Replace `consumer-command` with the user's real consumer; do not run a
placeholder command. Enable the shell's pipeline-failure propagation when it is
available so an exporter failure is not masked by the consumer. Never echo,
tee, log, preview, JSON-pretty-print, or parse secret stdout into displayed
output. Check the pipeline/export exit status before claiming success.

## Backup semantics

An export bundle is a point-in-time plaintext secret backup, not live sync.
OAuth refresh-token rotation can make an older bundle or the source machine's
stored token stale after another copy refreshes. Protect the bundle at rest,
transfer it only to the requested destination, and remove temporary copies.

For every auth command, inspect the exit status before parsing output.
`--json` controls successful output only; usage, validation, file, network, and
authentication errors can still be ordinary stderr text. Never treat an error
or normal text as JSON, and never include secret output in error diagnostics.
