## Summary

<!-- Brief description of the changes in this PR -->

### Changes

#### Breaking Changes
<!-- List any breaking changes, or remove this section if none -->
- None

#### New Features
<!-- List new features -->
-

#### Bug Fixes
<!-- List bug fixes -->
-

#### Test Coverage

| Package | Before | After | Change |
|---------|--------|-------|--------|
| | | | |

---

## Test Plan

- [ ] Unit tests pass (`make test`)
- [ ] Linting passes (`make lint`)
- [ ] Current lab dashboard checked when telemetry/UI changes (`make lab-dashboard`)
- [ ] Hook integration tested with Claude Code

## Checklist

- [ ] Code follows the project's style guidelines
- [ ] Self-review completed
- [ ] Tests added for new functionality
- [ ] Documentation updated (if applicable)
- [ ] CHANGELOG.md updated

## MLX integration evidence

For gateway, model, cache or protocol changes, link the relevant regression test
and update `docs/mlx-integration-verification.md` when its coverage changes.

- [ ] Model-free contract/race tests cover the changed behavior and its failure cases
- [ ] Real local Metal API proofs run, or explicitly reported as pending/skipped
- [ ] Tested Sentinel/MLX revisions and selected model families recorded
- [ ] Logical, reused and processed token counts remain distinct; unknown values stay unknown
- [ ] API proof artifacts use synthetic traffic and contain no private captures or credentials

Evidence (commands/results/artifact links):

## Related Issues

<!-- Link any related issues: Fixes #123, Relates to #456 -->
