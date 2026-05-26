<p align="center">
  <picture>
    <source srcset="docs/assets/logo-dark.svg" media="(prefers-color-scheme: dark)">
    <source srcset="docs/assets/logo-light.svg" media="(prefers-color-scheme: light)">
    <img src="docs/assets/logo-dark.svg" alt="agentpack" width="500">
  </picture>
</p>

<p align="center">The native package manager for <a href="https://agentskills.io">agentskills.io</a>.</p>

<p align="center">
  <a href="https://goreportcard.com/report/github.com/retr0h/agentpack"><img alt="go report card" src="https://goreportcard.com/badge/github.com/retr0h/agentpack?style=for-the-badge"></a>
  <a href="LICENSE"><img alt="license" src="https://img.shields.io/badge/license-MIT-brightgreen.svg?style=for-the-badge"></a>
  <a href="https://github.com/retr0h/agentpack/actions/workflows/go.yml"><img alt="build" src="https://img.shields.io/github/actions/workflow/status/retr0h/agentpack/go.yml?style=for-the-badge"></a>
  <a href="https://codecov.io/gh/retr0h/agentpack"><img alt="codecov" src="https://img.shields.io/codecov/c/github/retr0h/agentpack?style=for-the-badge"></a>
  <a href="https://conventionalcommits.org"><img alt="conventional commits" src="https://img.shields.io/badge/Conventional%20Commits-1.0.0-yellow.svg?style=for-the-badge"></a>
  <a href="https://just.systems"><img alt="built with just" src="https://img.shields.io/badge/Built_with-Just-black?style=for-the-badge&logo=just&logoColor=white"></a>
  <img alt="github commit activity" src="https://img.shields.io/github/commit-activity/m/retr0h/agentpack?style=for-the-badge">
  <a href="https://agentskills.io"><img alt="agentskills.io" src="https://img.shields.io/badge/standard-agentskills.io-ff6600?style=for-the-badge"></a>
</p>

<p align="center">
Install, manage, and distribute AI agent skills across Claude Code,
Cursor, Copilot, Codex, Gemini CLI, Windsurf, Goose, Roo, Amp, Cline,
and every agent supporting the <code>.agents/</code> convention.
</p>

## 📦 Install

```bash
curl -fsSL https://github.com/retr0h/agentpack/raw/main/install.sh | bash
```

### 🔨 Build from source

```bash
git clone https://github.com/retr0h/agentpack.git
cd agentpack
go build -o agentpack .
```

## 🚀 Quick Start

```bash
agentpack add github.com/org/skills-repo          # add from git
agentpack add github.com/org/repo#v1.0.0          # pin to a tag
agentpack add github.com/org/repo --skill review  # one skill only
agentpack ls                                      # list installed
agentpack ls --outdated                           # check for updates
agentpack info my-plugin                          # package details
agentpack info plugin.agentpack                   # peek inside an archive
agentpack verify plugin.agentpack                 # verify checksums
agentpack del my-plugin                           # delete a plugin
agentpack install                                 # install from agentpack-packages.yaml
agentpack build                                   # build .agentpack archives
```

See [Usage][] for full details.

## ✨ Features

- 📦 **[.agentpack format](docs/adr/001-agentpack-format.md)** — checksummed archives with skills, commands, hooks, MCP, agents, and settings
- 🤖 **50+ agents** — Claude Code, Cursor, Copilot, Codex, Gemini CLI, Windsurf, Goose, Roo, and more
- 🔒 **Content safety** — binary detection at build time, executable prompts at install
- 🔄 **Reproducible installs** — lockfile pins exact SHAs, `install` from manifest
- 🌐 **Global + local** — project-level or user-level installs (`-g`)
- ⚙️ **Config merging** — MCP servers, hooks, and settings merge into `.claude/settings.json`
- ✈️ **Offline** — `.agentpack` archives work without git, npm, or any toolchain
- 🔑 **Private repos** — `~/.netrc` and SSH key support
- 📋 **JSON everywhere** — `-o json` on every command for scripting

## 📖 Documentation

- [Usage][] — add, install, build, verify, info, examples
- [Package Format (ADR-001)][] — archive schema, content types
- [Architecture][] — driver design, install flow
- [Development][] — dev setup, testing conventions
- [Contributing][] — commit style, PR checklist

## 🙏 Acknowledgments

Agent detection paths and skill directory conventions inspired by
[vercel-labs/skills](https://github.com/vercel-labs/skills) and the
[agentskills.io](https://agentskills.io) ecosystem.

## 📄 License

[MIT][]

[MIT]: LICENSE
[Usage]: docs/usage.md
[Package Format (ADR-001)]: docs/adr/001-agentpack-format.md
[Architecture]: docs/architecture.md
[Development]: docs/development.md
[Contributing]: docs/contributing.md
