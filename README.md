[![go report card](https://goreportcard.com/badge/github.com/retr0h/agentpack?style=for-the-badge)](https://goreportcard.com/report/github.com/retr0h/agentpack)
[![license](https://img.shields.io/badge/license-MIT-brightgreen.svg?style=for-the-badge)](LICENSE)
[![build](https://img.shields.io/github/actions/workflow/status/retr0h/agentpack/go.yml?style=for-the-badge)](https://github.com/retr0h/agentpack/actions/workflows/go.yml)
[![codecov](https://img.shields.io/codecov/c/github/retr0h/agentpack?style=for-the-badge)](https://codecov.io/gh/retr0h/agentpack)
[![conventional commits](https://img.shields.io/badge/Conventional%20Commits-1.0.0-yellow.svg?style=for-the-badge)](https://conventionalcommits.org)
[![built with just](https://img.shields.io/badge/Built_with-Just-black?style=for-the-badge&logo=just&logoColor=white)](https://just.systems)
![github commit activity](https://img.shields.io/github/commit-activity/m/retr0h/agentpack?style=for-the-badge)
[![hovnokod](https://raw.githubusercontent.com/tekk/hovnokod-badge/main/assets/badges/hovnokod-for-the-badge.svg)](https://github.com/tekk/hovnokod-badge)

# agentpack

The first git-free package manager for [agentskills.io](https://agentskills.io).

Build checksummed `.agentpack` archives from any repo — skills, commands,
hooks, agents, MCP servers (remote and UX types), settings — and distribute
them via Google Drive, S3, URL, sneakernet, or git. Non-technical users
install with a single command. No git, Go, or build toolchain required on the
receiving end.

Works with Claude Code today. Cursor, Copilot, Gemini CLI, and other
agentskills.io-compatible platforms coming soon.

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
agentpack build                              # build all plugins in agentpack.yaml
agentpack build my-plugin                    # build one plugin
agentpack verify my-plugin-1.0.0.agentpack     # verify checksums
agentpack install my-plugin-1.0.0.agentpack    # install to ~/.claude/plugins/
agentpack install https://example.com/my-plugin-1.0.0.agentpack  # install from URL
agentpack list                               # show installed plugins
agentpack sync                               # sync from agentpack-packages.yaml
```

## ✨ Features

| Feature | Description |
|---------|-------------|
| Build | Package plugins from `agentpack.yaml` into `.agentpack` archives |
| Verify | SHA256 checksum verification of every file in an archive |
| Install | Unpack archives from local files or URLs |
| List | Show installed plugins with version, SHA, and install date |
| Sync | Declarative installs from `agentpack-packages.yaml` |
| Multi-plugin | One manifest, multiple plugins with src/dest remapping |
| MCP servers | Remote and UX/npx — agentpack generates `.mcp.json` |
| Git backend | Install directly from GitHub, GitLab, Bitbucket repos |
| Git metadata | Captures SHA, branch, timestamps at build time |
| Pluggable backends | File and HTTP today; S3 and GCS planned |
| Multi-agent targets | Claude Code today; Cursor, Copilot, Gemini CLI planned |
| agentskills.io | Compatible with the open agent skills ecosystem |

## 📋 Manifest

```yaml
name: my-plugin
version: 1.0.0
description: "My Claude Code plugin"

skills:
  - skills/*.md
commands:
  - commands/*.md
hooks:
  - hooks/hooks.json
  - hooks/*.sh
mcp:
  - type: remote
    name: my-api
    url: "https://mcp.example.com/v1"
  - type: ux
    name: my-server
    package: "@mycompany/my-mcp-server"
```

See [`examples/`](examples/) for single-plugin and multi-plugin manifests.

## 🔄 Declarative Sync

```yaml
# agentpack-packages.yaml — on the target machine
packages:
  - name: my-plugin
    source: https://drive.google.com/uc?id=1ABC&export=download
  - name: local-plugin
    source: ~/Downloads/local-plugin-1.0.0.agentpack
  - name: git-plugin
    git: github.com/myorg/my-plugin
  - name: git-plugin-pinned
    git: github.com/myorg/my-plugin
    ref: v2.0.0
```

```bash
agentpack sync
```

## 📖 Documentation

- [Architecture][] — driver design, install flow, content types
- [Package Format (ADR-001)][] — archive schema, content types, build pipeline
- [Development][] — dev setup, testing conventions, layout
- [Contributing][] — commit style, lint chain, PR checklist

## 📄 License

[MIT][]

[MIT]: LICENSE
[Architecture]: docs/architecture.md
[Package Format (ADR-001)]: docs/adr/001-agentpack-format.md
[Development]: docs/development.md
[Contributing]: docs/contributing.md
