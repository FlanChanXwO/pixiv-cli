# Inspect auth login syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv auth login --help
```

## Output
```text
Login with the Pixiv browser OAuth flow

Usage:
  pixiv auth login [flags]

Examples:
pixiv auth login --use

Flags:
      --addr string                  local loopback callback address; use 127.0.0.1:0 for an available port (default "127.0.0.1:0")
  -h, --help                         help for login
      --json                         print JSON
      --no-open                      do not open the browser
      --no-proxy                     clear the configured proxy for this command
      --proxy string                 proxy URL (http, https, socks5, or socks5h) for this command
      --relay-listen-addr string     listen address for this remote login relay
      --relay-public-url string      public URL for this remote login relay
      --relay-tls-cert-file string   PEM certificate file for this remote login relay
      --relay-tls-key-file string    PEM private key file for this remote login relay
      --timeout duration             maximum time to wait for login flow; 0 adds no deadline
      --use                          set as default account after login
```

Exit code: 0

Verdict: PASS
