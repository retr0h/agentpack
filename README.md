[![go report card](https://goreportcard.com/badge/github.com/retr0h/claudia?style=for-the-badge)](https://goreportcard.com/report/github.com/retr0h/claudia)
[![license](https://img.shields.io/badge/license-MIT-brightgreen.svg?style=for-the-badge)](LICENSE)
[![build](https://img.shields.io/github/actions/workflow/status/retr0h/claudia/go.yml?style=for-the-badge)](https://github.com/retr0h/claudia/actions/workflows/go.yml)
[![codecov](https://img.shields.io/codecov/c/github/retr0h/claudia?style=for-the-badge)](https://codecov.io/gh/retr0h/claudia)
[![conventional commits](https://img.shields.io/badge/Conventional%20Commits-1.0.0-yellow.svg?style=for-the-badge)](https://conventionalcommits.org)
[![built with just](https://img.shields.io/badge/Built_with-Just-black?style=for-the-badge&logo=just&logoColor=white)](https://just.systems)
![macOS](https://img.shields.io/badge/macOS-000000?style=for-the-badge&logo=apple&logoColor=white)
![github commit activity](https://img.shields.io/github/commit-activity/m/retr0h/claudia?style=for-the-badge)

<p align="center">A git-free package manager for Claude Code plugins.</p>

Build, checksum, and distribute Claude Code plugin archives so non-technical
users can install skills, commands, hooks, agents, MCP servers, and settings
without needing git, Go, or any build toolchain.

## Features

- **Build** archives from a `claudia.yaml` manifest
- **Per-file SHA256** checksums baked into every archive
- **Verify** archive integrity before installation
- **Skills, commands, hooks, agents** -- markdown-based plugin content
- **MCP servers** -- pre-built binaries, remote endpoints, or UX/npx packages
- **Settings fragments** -- JSON merged into Claude Code's `settings.json`
- **Git metadata** -- captures SHA, version, timestamps at build time
- **Claude Code native** -- installs into `~/.claude/plugins/` using the same
  directory structure as the official marketplace

## Install

```bash
curl -fsSL https://github.com/retr0h/claudia/raw/main/install.sh | bash
```

<details>
<summary>Build from source</summary>

```bash
git clone https://github.com/retr0h/claudia.git
cd claudia
go build -o claudia .
install -m 755 claudia ~/.local/bin/claudia
```

</details>

## Usage

### Building a package

Create a `claudia.yaml` in your plugin repo:

```yaml
name: my-plugin
version: 1.0.0
description: My Claude Code plugin

skills:
  - skills/*.md

commands:
  - commands/*.md

hooks:
  - hooks/hooks.json
  - hooks/*.sh

agents:
  - agents/*.md

mcp:
  - type: binary
    src: bin/my-server
    platforms: [darwin-arm64, darwin-amd64, linux-amd64]

  - type: remote
    config: .mcp.json

  - type: ux
    package: "@mycompany/my-mcp-server"

binaries:
  - bin/my-tool

settings:
  - settings.json
```

Then build:

```bash
claudia build
```

This produces `my-plugin-1.0.0.claudia` -- a checksummed tarball ready to
distribute via Google Drive, S3, email, or any file transfer.

### Verifying a package

```bash
claudia verify my-plugin-1.0.0.claudia
```

Checks every file against its SHA256 checksum in the embedded `checksums.txt`.

### Installing a package (planned)

```bash
claudia install my-plugin-1.0.0.claudia
```

Unpacks into `~/.claude/plugins/` matching Claude Code's native marketplace
directory structure.

## Package contents

A `.claudia` archive can contain any combination of:

| Content | Format | Description |
|---------|--------|-------------|
| Skills | `.md` | Markdown skill files |
| Commands | `.md` | Slash commands referenced in `plugin.json` |
| Hooks | `.json` + `.sh` | Hook definitions and scripts |
| Agents | `.md` | Agent definitions |
| MCP (binary) | executable | Pre-built MCP server binaries |
| MCP (remote) | `.mcp.json` | Config pointing to a remote MCP endpoint |
| MCP (ux/npx) | config | References a UX package |
| Binaries | executable | Pre-built executables included as-is |
| Settings | `.json` | Fragments merged into `settings.json` |

## Requirements

- macOS or Linux
- Go 1.25+ (to build from source)

## License

The [MIT][] License.

[MIT]: LICENSE
