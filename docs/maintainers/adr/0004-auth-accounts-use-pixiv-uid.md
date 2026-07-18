# ADR 0004: Auth Accounts Use Pixiv UID

## Status

Accepted.

## Context

Local auth previously let users choose arbitrary account names. That made CLI prompts convenient, but the stored account identity was not the Pixiv identity. As login and token import both learn the Pixiv user ID after successful authentication, asking users to invent another name adds friction and creates duplicate identity terms.

The project needs local account selection to stay scriptable while matching Pixiv's real identity model.

## Decision

- Store local auth accounts by Pixiv `user_id`.
- Use `default_user_id` as the default account pointer in `auth.json`.
- Keep optional `username` only as display metadata.
- Remove name prompts from `pixiv auth add` and `pixiv auth login`.
- Use UID selectors for `pixiv auth use/remove/check`.
- Add canonical `--uid`; keep `--profile` only as a deprecated alias.
- Treat old `default_account/accounts[].name` auth files as incompatible and require users to recreate auth state with `pixiv auth add` or `pixiv auth login`.

## Consequences

- CLI auth commands require fewer manual inputs and cannot drift from Pixiv identity.
- Existing scripts that pass custom auth names must switch to UID.
- Existing `auth.json` files using custom names must be recreated.
- Username changes do not affect account selection because username is not the key.
- Application and storage layers no longer depend on CLI-specific profile terminology.
