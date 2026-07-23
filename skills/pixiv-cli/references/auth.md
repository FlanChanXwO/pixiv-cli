# Authentication import and export

Use these workflows only when the user explicitly asks to import, export, back
up, or restore authentication. Refresh tokens and every export bundle are
plaintext secrets.

## Browser OAuth login

Run `pixiv auth login` only on the user's explicit request while they are
present to approve the Pixiv page. On macOS, desktop Linux, and Windows, the
CLI temporarily owns the `pixiv://` callback association for that one login;
it restores the prior user association on completion, failure, or cancellation.
Do not invoke the hidden protocol-handler helper directly.

For a GUI-less SSH server, keep the existing public interface: run
`pixiv auth login --no-open --addr 127.0.0.1:PORT` on the server and
`ssh -N -L PORT:127.0.0.1:PORT HOST` on the browser machine. Open the forwarded
fallback page locally. Submit a Pixiv relay or final callback there; an accepted
relay continues in that same browser and the final submission returns through
the tunnel to the server loopback listener. Do not bind the listener publicly,
do not require pixiv on the browser machine, and do not read browser state or
try to automate the authorization page.

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
pixiv auth import --file /private/path/pixiv-auth.json
pixiv auth import --file - < /private/path/pixiv-auth.json
```

`--file PATH` and `--file -` restore a versioned bundle entirely offline. A
file import cannot be combined with a positional token, `--proxy`, or
`--no-proxy`; proxy flags do not apply to bundle restore. The bundle itself
contains plaintext refresh tokens, even though normal text/JSON success output
and errors do not echo them. Do not inspect, summarize, or log bundle content.

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
