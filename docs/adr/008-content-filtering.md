---
status: accepted
date: 2026-05-25
---

# ADR-008: Content Type Filtering in Manifest

## Context and Problem Statement

When a repo ships hooks or MCP configs alongside skills, `add repo@skill`
installs the skill but ALSO installs all hooks and MCP configs. Users have no
way to say "give me the skills but not the hooks." The CLI shorthand should stay
simple (`@skill` = skills only). The manifest should provide full control.

## Decision Drivers

- Repos can ship skills, commands, hooks, MCP, settings, and agents
- Users may want skills but not hooks that modify config
- CLI shorthand (`@skill`) only handles skills — need full control somewhere
- The manifest (`agentpack-packages.yaml`) is the right place for declarative
  control

## Considered Options

- **CLI flags per content type** (`--no-hooks`, `--include commands`) — clutters
  the CLI, hard to remember, not reproducible across machines
- **Separate install commands per type** — breaks the single-package model
- **Always install everything** — current behavior, no control over side effects
  like config merging from unwanted hooks/MCP

## Decision Outcome

### CLI behavior (no change)

- `add owner/repo@skill` — installs only that skill (skills content type)
- `add owner/repo --skill foo --skill bar` — installs named skills
- `add owner/repo` — installs everything from the repo

The `@skill` shorthand implies `content: [skills]` — only skills are installed,
other content types are skipped.

### Manifest per-type selectors

Each content type gets its own optional field in `agentpack-packages.yaml`. When
a field is present, only the named items are installed for that type. When
absent, all items of that type are installed.

```yaml
packages:
  # Pick specific items from each content type
  - name: security-tools
    git: github.com/org/security-skills
    skills:
      - code-review
      - dependency-audit
    commands:
      - scan

  # Pick only an agent, nothing else
  - name: agent-only
    git: github.com/org/toolkit
    agents:
      - security-reviewer
    skills: [] # explicit empty = skip all skills
    commands: [] # explicit empty = skip all commands

  # Install everything (current behavior, backward compatible)
  - name: full-toolkit
    git: github.com/org/toolkit
```

Supported filter fields:

| Field      | Filters                  | Directory   |
| ---------- | ------------------------ | ----------- |
| `skills`   | Skill subdirectory names | `skills/`   |
| `commands` | Command file names       | `commands/` |
| `agents`   | Agent subdirectory names | `agents/`   |
| `hooks`    | Hook file names          | `hooks/`    |
| `mcp`      | MCP config names         | `mcp/`      |
| `settings` | Settings file names      | `settings/` |

An explicit empty list (`skills: []`) means "install none of this type."
Omitting the field entirely means "install all of this type."

### Interaction with config merging

When `hooks` or `mcp` or `settings` are excluded from the `content` whitelist:

- The files are not copied
- Config merging does NOT run for those types
- No entries are written to `.claude/settings.json`

This is safe because the content check happens before the target install, not
after.

### Interaction with `@skill` shorthand

When `@skill` is used on the CLI, it implicitly sets `content: [skills]` for
that install. This means:

- Only skills are packaged and installed
- Commands, hooks, MCP, settings from the repo are ignored
- No config merging side effects

### `content` in install Options

```go
type Options struct {
    // ...existing fields...
    Content []string // whitelist of content types to install
}
```

When empty, all content types are included. The auto-packager
(`autoPackageWithVersion`) filters `contentDirs` against this list.

## Consequences

- Users can install skills without side effects from hooks/MCP
- `@skill` becomes safe by default — no config modifications
- Full control via manifest for reproducible, auditable installs
- Backward compatible — omitting `content` installs everything
- Content type names match the archive directory names (ADR-001)
