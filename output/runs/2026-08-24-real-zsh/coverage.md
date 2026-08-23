# Coverage

## Task 14 baseline

| Capability | Case | Status | Evidence |
| --- | --- | --- | --- |
| Real Pixiv CLI process | `baseline/version` | PASS | The runner invokes a binary named `pixiv` directly rather than evaluating a shell command string. |
| Independent command cwd | `baseline/version` | PASS | The case runs from `/private/tmp/pixiv-cli-e2e-shell-20260821`, outside `output/`. |
| Raw stream separation | `baseline/version` | PASS | Pixiv CLI stdout and stderr are stored independently. |

## Pending Task 15

The comprehensive Pixiv read-only, authentication-diagnostic, download, representative error-path, current-user route, latest-timeline continuation, and thumbnail MIME/extension cases are intentionally pending. They are not claimed as covered by this Task 14 baseline.
