# Unreleased

## Changed

- Documentation-only pull requests and `main` pushes now run the documentation contract check instead of the full Linux quality gate and six-platform packaged-binary smoke. Any source, dependency, script, or workflow change still runs the complete validation set.
- Data commands now use the selected local account or an optional manual account pool; they reject per-command UID/token selection and ignore `PIXIV_REFRESH_TOKEN`. Their streaming `--ndjson` records have stable `id`, `type`, and `url` fields, and `filter` plus download/bookmark/follow actions can consume the same protocol.
- `pixiv config` now manages only `download_path`, `filename_template`, and `https_proxy`. Advanced settings remain private hand-maintained TOML.
- Text-mode data commands now report stdout write failures instead of returning success after a partial result.

## Added

- Ugoira downloads can produce APNG with `--ugoira-format apng`; GIF remains the default.
- The public SDK and MCP entity-reading tools expose canonical records, including feeds and MyPixiv reads. MCP text is now a short summary rather than a duplicate entity payload.
- Published release tags can now publish the product skill to ClawHub as well as SkillHub. The ClawHub workflow verifies the immutable release source, performs a credential-free dry-run, and supplies `CLAWHUB_TOKEN` only to its final publish step.
