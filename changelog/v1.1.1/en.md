# v1.1.1 — 2026-09-23

## Changed

- A release now needs only one final approval. Builds, end-to-end SDK checks, source and artifact verification, production builds, and container checks run automatically; after that approval, the GitHub Release and downstream packages are published. ([#103](https://github.com/FlanChanXwO/pixiv-cli/pull/103), [#104](https://github.com/FlanChanXwO/pixiv-cli/pull/104), [#108](https://github.com/FlanChanXwO/pixiv-cli/pull/108), [#121](https://github.com/FlanChanXwO/pixiv-cli/pull/121))
- ClawHub publication waits until the released skill is visible in the registry before verifying it. ([#87](https://github.com/FlanChanXwO/pixiv-cli/pull/87))

## Security

- Automatic pull request checks run against the actual head commit and keep their names stable, so a fork or a changed base branch cannot reuse old results. ([#100](https://github.com/FlanChanXwO/pixiv-cli/pull/100), [#101](https://github.com/FlanChanXwO/pixiv-cli/pull/101), [#102](https://github.com/FlanChanXwO/pixiv-cli/pull/102), [#107](https://github.com/FlanChanXwO/pixiv-cli/pull/107), [#111](https://github.com/FlanChanXwO/pixiv-cli/pull/111), [#115](https://github.com/FlanChanXwO/pixiv-cli/pull/115), [#117](https://github.com/FlanChanXwO/pixiv-cli/pull/117), [#119](https://github.com/FlanChanXwO/pixiv-cli/pull/119), [#120](https://github.com/FlanChanXwO/pixiv-cli/pull/120), [#88](https://github.com/FlanChanXwO/pixiv-cli/pull/88), [#89](https://github.com/FlanChanXwO/pixiv-cli/pull/89), [#90](https://github.com/FlanChanXwO/pixiv-cli/pull/90), [#91](https://github.com/FlanChanXwO/pixiv-cli/pull/91), [#93](https://github.com/FlanChanXwO/pixiv-cli/pull/93), [#94](https://github.com/FlanChanXwO/pixiv-cli/pull/94), [#99](https://github.com/FlanChanXwO/pixiv-cli/pull/99))

**Full Changelog**: [v1.1.0...v1.1.1](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.0...v1.1.1)

<!-- internal:
  [#105](https://github.com/FlanChanXwO/pixiv-cli/pull/105)
  [#109](https://github.com/FlanChanXwO/pixiv-cli/pull/109)
  [#110](https://github.com/FlanChanXwO/pixiv-cli/pull/110)
  [#114](https://github.com/FlanChanXwO/pixiv-cli/pull/114)
  [#92](https://github.com/FlanChanXwO/pixiv-cli/pull/92)
  [#96](https://github.com/FlanChanXwO/pixiv-cli/pull/96)
  [#97](https://github.com/FlanChanXwO/pixiv-cli/pull/97)
  [#98](https://github.com/FlanChanXwO/pixiv-cli/pull/98)
-->
