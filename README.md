<p align="center">
  <picture>
    <source srcset="docs/assets/logo-dark.svg" media="(prefers-color-scheme: dark)">
    <source srcset="docs/assets/logo-light.svg" media="(prefers-color-scheme: light)">
    <img src="docs/assets/logo-dark.svg" alt="agentpack" width="500">
  </picture>
</p>

<p align="center">An open package format for AI agent skills.</p>

<p align="center">
  <a href="https://github.com/retr0h/agentpack/releases/latest"><img alt="release" src="https://img.shields.io/github/release/retr0h/agentpack.svg?style=for-the-badge"></a>
  <a href="https://goreportcard.com/report/github.com/retr0h/agentpack"><img alt="go report card" src="https://goreportcard.com/badge/github.com/retr0h/agentpack?style=for-the-badge"></a>
  <a href="LICENSE"><img alt="license" src="https://img.shields.io/badge/license-MIT-brightgreen.svg?style=for-the-badge"></a>
  <a href="https://github.com/retr0h/agentpack/actions/workflows/go.yml"><img alt="build" src="https://img.shields.io/github/actions/workflow/status/retr0h/agentpack/go.yml?style=for-the-badge"></a>
  <a href="https://codecov.io/gh/retr0h/agentpack"><img alt="codecov" src="https://img.shields.io/codecov/c/github/retr0h/agentpack?style=for-the-badge"></a>
  <a href="https://conventionalcommits.org"><img alt="conventional commits" src="https://img.shields.io/badge/Conventional%20Commits-1.0.0-yellow.svg?style=for-the-badge"></a>
  <a href="https://just.systems"><img alt="built with just" src="https://img.shields.io/badge/Built_with-Just-black?style=for-the-badge&logo=just&logoColor=white"></a>
  <img alt="github commit activity" src="https://img.shields.io/github/commit-activity/m/retr0h/agentpack?style=for-the-badge">
  <a href="https://agentskills.io"><img alt="agentskills.io" src="https://img.shields.io/badge/standard-agentskills.io-ff6600?style=for-the-badge"></a>
</p>

## One package, every agent

AI coding agents each have their own conventions for skills, rules, commands,
hooks, and MCP servers. A skill that works in Claude Code doesn't work in
Cursor. A hook that works in Cline doesn't exist in Codex.

The `.agentpack` format solves this. A single archive describes what content it
contains — skills, commands, hooks, agents, MCP integrations, config — and each
agent's driver installs only what it supports, wherever it belongs.

```
.agentpack archive                    Installs to
┌───────────────────┐
│ metadata.yaml     │
│                   │
│  k8s     skill   ─┼──→  Claude Code  .claude/skills/k8s/
│                  ─┼──→  Cursor       .cursor/rules/k8s/
│                  ─┼──→  Codex        .codex/skills/k8s/
│                   │
│  scan    command ─┼──→  Claude Code  .claude/commands/scan/
│                   │
│  my-api  mcp     ─┼──→  Claude Code  .claude/settings.json (mcpServers)
│                   │
│  on-save hook    ─┼──→  Claude Code  .claude/settings.json (hooks)
│                   │
│  theme   config  ─┼──→  Claude Code  .claude/settings.json
│                   │
│                   │     Cursor, Codex skip unsupported types
└───────────────────┘
```

Package authors write content once. The format handles the rest.

## The .agentpack format

A `.agentpack` file is a gzipped tarball with typed metadata. Six content types
cover the AI agent ecosystem:

| Type | What it is | Who supports it |
|------|-----------|----------------|
| **skill** | Knowledge/capability module | All agents |
| **command** | User-invoked action | Claude Code |
| **hook** | Event-driven automation | Claude Code |
| **agent** | Subagent/persona definition | Claude Code |
| **mcp** | External service integration | Claude Code |
| **config** | Configuration the package needs | Claude Code |

Each agent driver declares which types it supports. When an agent adds support
for a new type, existing packages automatically start working — no package
changes needed.

The format is designed to become an open standard. See
[ADR-009: Metadata-Driven Package Format][Format] for the full specification.

## Quick start

```bash
# Install agentpack
curl -fsSL https://github.com/retr0h/agentpack/raw/main/install.sh | bash

# Find and install skills
agentpack search react
agentpack add owner/repo@skill-name
agentpack add owner/repo --target claude-code
agentpack add owner/repo -g                     # install globally

# Manage packages
agentpack ls                                    # list installed
agentpack ls --targets                          # show detected agents
agentpack info owner/repo                       # package details
agentpack del owner/repo@skill-name             # remove a skill
agentpack del owner/repo                        # remove entire package

# Author packages
agentpack init my-skill                         # scaffold a new skill
agentpack build                                 # build .agentpack archive
agentpack install                               # install from manifest
```

See [Usage][] for full details.

## 50+ supported agents

Claude Code, Cursor, Copilot, Codex, Gemini CLI, Windsurf, Cline, Goose, Roo,
Amp, Continue, Kiro, Devin, Warp, Trae, and every agent supporting the
`.agents/` convention. See `agentpack ls --targets` for the full list.

## Why not just clone git repos?

Tools like `npx @anthropic-ai/claude-code skills` clone repos and copy files.
That works for one agent. It breaks down when you need:

- **Multiple agents** — each agent has different paths. The format handles translation.
- **Selective content** — install one skill from a repo with 50. `@skill` filtering.
- **Reproducibility** — lockfile pins exact git SHAs. `agentpack install` is deterministic.
- **Safe removal** — registry tracks every file. `del` removes exactly what was installed.
- **Content safety** — binary detection at build time, executable prompts at install.
- **Offline installs** — `.agentpack` archives work without git or any toolchain.

The `.agentpack` format is the layer that makes all of this possible. It could
be adopted by any tool — including npx skills — as the standard packaging for
AI agent content.

## Documentation

- [Usage][] — add, install, build, verify, info, examples
- [Format (ADR-009)][Format] — the .agentpack specification
- [Architecture][] — driver design, install flow, capability model
- [Development][] — dev setup, testing conventions
- [Contributing][] — commit style, PR checklist

## Acknowledgments

Agent detection paths and skill directory conventions inspired by
[vercel-labs/skills](https://github.com/vercel-labs/skills) and the
[agentskills.io](https://agentskills.io) ecosystem.

## License

[MIT][]

[MIT]: LICENSE
[Usage]: docs/usage.md
[Format]: docs/adr/009-metadata-driven-format.md
[Architecture]: docs/architecture.md
[Development]: docs/development.md
[Contributing]: docs/contributing.md
