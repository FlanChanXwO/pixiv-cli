# Unreleased

## Fixed

- Trigger the automatic SkillHub publication from a successful Release workflow instead of its GitHub Release event, because releases created with `github.token` do not recursively emit that event to Actions.
