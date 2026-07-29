# v0.8.0 Final Report

## Delivered

- Integrated the non-Fanbox/non-login-page v0.8 work into the service-oriented
  public SDK, application, CLI and MCP boundaries.
- Added feeds, canonical NDJSON Records, local filter/action pipelines, manual
  account-pool state and safe Retry-After switching.
- Made data CLI commands use local selected accounts only; `PIXIV_REFRESH_TOKEN`
  and per-command credential selectors are excluded from this contract.
- Restricted `pixiv config` to `download_path`, `filename_template`, and
  `https_proxy`.
- Added GIF-default/APNG-explicit ugoira downloads through the shared encoder.
- Added immutable-release ClawHub publishing alongside SkillHub and synchronized
  product skill, three-locale references, MCP/SDK docs and changelogs.

## Quality closure

Three independent reviews were completed. The last review found no in-scope
Critical, Major, P1 or Minor issue. Writer failures now return errors and are
treated as committed before any possible stdout byte, preventing a typed
downstream 429 from replaying an account-pool read. The offline binary E2E also
tests the final three-alias config boundary.

## Verification

- `go test ./... -count=1`
- `go test -race ./...`
- `go vet ./...`
- `go test ./e2e -count=1`
- `go test ./scripts/clawhubworkflow ./scripts/releaseworkflow ./scripts/documentation -count=1`
- `sh scripts/build.sh`
- `sh scripts/test-release-workflow.sh`
- `sh scripts/test-homebrew-prepublish-workflow.sh`
- `pre-commit run --all-files`
- `git diff --check`

## Release-evidence limitation

The real CLI Web fallback E2E was attempted with the local proxy and directly.
Pixiv reset the proxy path and timed out directly. It neither logged in nor
mutated remote data, but it did not reach a successful live assertion. The real
authenticated CLI canaries require the protected release environment and test
token, so they remain a release-environment gate rather than local evidence.
