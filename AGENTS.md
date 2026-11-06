# AGENTS.md

Test: `just test` | Before committing: `just ready`

Read [docs/development.md](docs/development.md) first. It covers prerequisites,
common tasks, code conventions, testing conventions, mocks, driver development,
theming and commit messages. [docs/contributing.md](docs/contributing.md) covers
branching and pull requests. All of it applies to agents exactly as it applies
to people. This file carries only what is specific to agents.

## Running tools

Invoke tools through `mise`, not from your path:

```bash
mise exec -- just test
```

`mise` is active in a person's shell and supplies the versions `.mise.toml`
declares. An agent's shell has no activation, so a bare `just` resolves to
whatever is installed globally, usually an older version.

The symptom is a check that fails here and passes in continuous integration, on
a file nobody edited. When that happens, establish which version ran before
treating the failure as real.

## Shell commands

Claude Code's permission parser cannot analyse some shell syntax, and halts for
a manual prompt when it meets it. Prefer the Read, Grep, Edit and Write tools,
which never prompt. When you do shell out:

- No shell variables and no command substitution. Run the command that produces
  a value on its own, read the result, and put the literal into the next
  command.
- No redirections or pipes (`>`, `>>`, `<`, `2>&1`). Pass paths as arguments and
  let stdout and stderr reach the transcript.
- No `#` comments inside a multi-line quoted argument such as `python -c`. Write
  the script to a file instead.
- Use `git -C path` rather than `cd path && git`.
- Keep scratch files inside the repository, under `./.tmp/`, which is ignored. A
  path outside the project always prompts, whatever the allow-list says.

## Commit trailer

When committing via Claude Code, end the message with:

```
🤖 Generated with [Claude Code](https://claude.ai/code)

Co-Authored-By: Claude <noreply@anthropic.com>
```
