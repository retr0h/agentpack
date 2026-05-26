# Contributing to agentpack

Thanks for your interest in contributing.

## Before you start

- **Read [AI_POLICY.md](../AI_POLICY.md)** -- disclose AI assistance, ensure you
  understand any code you submit.
- **Check existing work** -- search open issues and PRs to avoid duplicating
  effort.
- **Start small** -- focused PRs are easier to review than sweeping changes.

## Making changes

### Code style

- Run `gofmt -l .` and `go vet ./...` before pushing.
- Tests: `go test ./...`. Add tests for any non-trivial behavior.
- Idiomatic Go -- see [development.md](development.md).

### Documentation

- Update docs alongside code changes when public behavior changes.
- Write an ADR in `docs/adr/` for significant design decisions (archive format,
  new targets, security model changes).

## Submitting a PR

1. Create a feature branch from `main` (`type/short-description` -- `feat/`,
   `fix/`, `docs/`, `refactor/`, `chore/`).
2. Commit messages:
   [Conventional Commits](https://www.conventionalcommits.org/), see
   [development.md](development.md#commit-messages).
3. PR description: what changed, why, and any follow-ups.
4. Open as draft if you want early feedback before final review.
5. One logical change per PR -- split unrelated changes.
