[![go report card](https://goreportcard.com/badge/github.com/retr0h/agentpack?style=for-the-badge)](https://goreportcard.com/report/github.com/retr0h/agentpack)
[![license](https://img.shields.io/badge/license-MIT-brightgreen.svg?style=for-the-badge)](LICENSE)
[![build](https://img.shields.io/github/actions/workflow/status/retr0h/agentpack/go.yml?style=for-the-badge)](https://github.com/retr0h/agentpack/actions/workflows/go.yml)
[![codecov](https://img.shields.io/codecov/c/github/retr0h/agentpack?style=for-the-badge)](https://codecov.io/gh/retr0h/agentpack)
[![conventional commits](https://img.shields.io/badge/Conventional%20Commits-1.0.0-yellow.svg?style=for-the-badge)](https://conventionalcommits.org)
[![built with just](https://img.shields.io/badge/Built_with-Just-black?style=for-the-badge&logo=just&logoColor=white)](https://just.systems)
![github commit activity](https://img.shields.io/github/commit-activity/m/retr0h/agentpack?style=for-the-badge)
[![hovnokod](https://raw.githubusercontent.com/tekk/hovnokod-badge/main/assets/badges/hovnokod-for-the-badge.svg)](https://github.com/tekk/hovnokod-badge)

# agentpack

The native package manager for [agentskills.io](https://agentskills.io).

Install, manage, and distribute AI agent skills across Claude Code,
Cursor, Copilot, Gemini CLI, Codex, OpenCode, and Windsurf. Works with
any [officialskills.sh](https://officialskills.sh) repo out of the box.

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
agentpack install github.com/org/skills-repo          # install from git
agentpack install github.com/org/repo#v1.0.0          # pin to a tag
agentpack install github.com/org/repo --skill review  # one skill only
agentpack list                                        # show installed
agentpack show my-plugin                              # package details
agentpack inspect plugin.agentpack                    # peek inside an archive
agentpack verify plugin.agentpack                     # verify checksums
agentpack update my-plugin                            # update to latest
agentpack outdated                                    # check for updates
agentpack remove my-plugin                            # uninstall
agentpack sync                                        # sync from agentpack.yaml
```

See [Usage][] for full details.

## ✨ Features

| Feature | Description |
|---------|-------------|
| Install from git | Any GitHub/GitLab/Bitbucket repo, with `--skill` / `--agent` filters |
| Install from archive | `.agentpack` files via local path, URL, or offline transfer |
| Build packages | `agentpack build` from `agentpack.yaml` manifest |
| Inspect | Peek inside `.agentpack` archives — metadata, files, checksums |
| Verify | SHA256 checksum verification — internal + external (`--sha256`) |
| List / Show | See what's installed, source, SHA, every file tracked |
| Update / Outdated | Check for and apply updates from git sources |
| Remove | Safe uninstall — only deletes files agentpack installed |
| Multi-agent | Claude Code, Cursor, Copilot, Gemini CLI, Codex, OpenCode, Windsurf |
| officialskills.sh | Native support for the agentskills.io skill format |
| Offline | `.agentpack` archives work without git, npm, or any toolchain |
| Netrc | Private repo support via `~/.netrc` credentials |

## 📖 Documentation

- [Usage][] — install, build, sync, verify, inspect, examples
- [Package Format (ADR-001)][] — archive schema, content types
- [Architecture][] — driver design, install flow
- [Development][] — dev setup, testing conventions
- [Contributing][] — commit style, PR checklist

## 📄 License

[MIT][]

[MIT]: LICENSE
[Usage]: docs/usage.md
[Package Format (ADR-001)]: docs/adr/001-agentpack-format.md
[Architecture]: docs/architecture.md
[Development]: docs/development.md
[Contributing]: docs/contributing.md
