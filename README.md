[![go report card](https://goreportcard.com/badge/github.com/retr0h/claudia?style=for-the-badge)](https://goreportcard.com/report/github.com/retr0h/claudia)
[![license](https://img.shields.io/badge/license-MIT-brightgreen.svg?style=for-the-badge)](LICENSE)
[![build](https://img.shields.io/github/actions/workflow/status/retr0h/claudia/go.yml?style=for-the-badge)](https://github.com/retr0h/claudia/actions/workflows/go.yml)
[![codecov](https://img.shields.io/codecov/c/github/retr0h/claudia?style=for-the-badge)](https://codecov.io/gh/retr0h/claudia)
[![conventional commits](https://img.shields.io/badge/Conventional%20Commits-1.0.0-yellow.svg?style=for-the-badge)](https://conventionalcommits.org)
[![built with just](https://img.shields.io/badge/Built_with-Just-black?style=for-the-badge&logo=just&logoColor=white)](https://just.systems)
![github commit activity](https://img.shields.io/github/commit-activity/m/retr0h/claudia?style=for-the-badge)
[![hovnokod](https://raw.githubusercontent.com/tekk/hovnokod-badge/main/assets/badges/hovnokod-for-the-badge.svg)](https://github.com/tekk/hovnokod-badge)

# claudia

The first git-free package manager for [agentskills.io](https://agentskills.io).

Build checksummed `.claudia` archives from any repo — skills, commands,
hooks, agents, MCP servers, binaries, settings — and distribute them
via Google Drive, S3, URL, or sneakernet. Non-technical users install
with a single command. No git, Go, or build toolchain required on the
receiving end.

Works with Claude Code today. Cursor, Copilot, Gemini CLI, and other
agentskills.io-compatible platforms coming soon.

## 📦 Install

```bash
curl -fsSL https://github.com/retr0h/claudia/raw/main/install.sh | bash
```

### 🔨 Build from source

```bash
git clone https://github.com/retr0h/claudia.git
cd claudia
go build -o claudia .
```

## 🚀 Quick Start

```bash
claudia build                              # build all plugins in claudia.yaml
claudia build my-plugin                    # build one plugin
claudia verify my-plugin-1.0.0.claudia     # verify checksums
claudia install my-plugin-1.0.0.claudia    # install to ~/.claude/plugins/
claudia install https://example.com/p.claudia  # install from URL
claudia list                               # show installed plugins
claudia sync                               # sync from claudia-packages.yaml
```

## ✨ Features

| Feature | Description |
|---------|-------------|
| Build | Package plugins from `claudia.yaml` into `.claudia` archives |
| Verify | SHA256 checksum verification of every file in an archive |
| Install | Unpack archives from local files or URLs |
| List | Show installed plugins with version, SHA, and install date |
| Sync | Declarative installs from `claudia-packages.yaml` |
| Multi-plugin | One manifest, multiple plugins with src/dest remapping |
| MCP servers | Binary, remote, and UX/npx — claudia generates `.mcp.json` |
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
  - type: binary
    name: my-server
    src: bin/my-server
  - type: remote
    name: my-api
    url: "https://mcp.example.com/v1"
```

See [`examples/`](examples/) for single-plugin and multi-plugin manifests.

## 🔄 Declarative Sync

```yaml
# claudia-packages.yaml — on the target machine
packages:
  - name: my-plugin
    source: https://drive.google.com/uc?id=1ABC&export=download
  - name: local-plugin
    source: ~/Downloads/local-plugin-1.0.0.claudia
```

```bash
claudia sync
```

## 📖 Documentation

- [Package Format][] — archive layout, generated files, MCP config
- [Development][] — architecture, testing conventions, layout
- [Contributing][] — commit style, lint chain, PR checklist

## 📄 License

[MIT][]

[MIT]: LICENSE
[Package Format]: docs/package-format.md
[Development]: docs/development.md
[Contributing]: docs/contributing.md
