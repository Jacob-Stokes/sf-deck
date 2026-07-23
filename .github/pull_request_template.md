<!--
Thanks for the PR.

For anything bigger than a one-line fix, please open an issue first
so we can agree on the direction before code lands.

A few patterns worth respecting — see `.github/CONTRIBUTING.md` for the full
list:

- New verbs go in internal/verbs/registry.go FIRST. The drift test
  will catch you if the implementation isn't there.
- Writes declare their required safety level on the registry Spec.
- The CLI/IPC JSON envelope contract is stable. Don't break it.
- Comments explain WHY, not WHAT.
- No emojis in code or commit messages.

Tick what applies. Strike (e.g. ~~docs~~) the ones that don't.
-->

## Summary

<!-- 1-3 sentences. What does this PR do and why? -->

## Type of change

- [ ] Bug fix
- [ ] New feature
- [ ] Behaviour change
- [ ] Internal refactor (no behaviour change)
- [ ] Documentation only

## Checks

- [ ] `go test ./...` passes
- [ ] `go vet ./...` clean
- [ ] If a verb was added or changed: `internal/verbs/registry.go` updated
- [ ] If the registry changed: `go run ./cmd/sf-deck-docs` re-run
- [ ] If a write verb was added or modified: safety gate level set + tested
- [ ] If user-facing behaviour changed: relevant docs updated (`docs/user/...`)
- [ ] Commit messages explain WHY, not WHAT

## Test plan

<!--
How did you verify this? Mention any specific orgs / fixtures /
manual steps. "ran tests" alone is fine for small refactors but
not for behaviour changes.
-->
