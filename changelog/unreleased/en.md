# Unreleased

## Fixed

- Trigger the automatic SkillHub publication from a successful Release workflow instead of its GitHub Release event, because releases created with `github.token` do not recursively emit that event to Actions.
- Accept SkillHub's community `reviewStatus` response field when confirming an accepted submission, so a successful publish is not reported as failed.
