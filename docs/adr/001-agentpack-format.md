# ADR-001: .agentpack Package Format

## Status

Accepted

## Decision Drivers

- Must work offline — no network required after initial fetch
- Must be verifiable — tamper and corruption detection
- No binary executables — security policy
- Format must be agent-agnostic — one archive, many targets
- Non-technical users shouldn't need git, Go, or npm to install

## Considered Alternatives

- **npm packages** — requires Node.js runtime, heavyweight for markdown files
  and JSON config
- **Git submodules** — requires git, complex UX, no checksums, no offline story
- **Raw file copy (no packaging)** — no verification, no metadata, no
  versioning, no offline transfer
- **Container images (OCI)** — massive overkill for config files and markdown
  skills

## Context

AI coding agents (Claude Code, Cursor, Copilot, Gemini CLI, Codex, OpenCode,
Windsurf) need a standard way to distribute and install skills, commands, hooks,
agents, MCP configs, and settings. There is no existing package format for this
— agentskills.io defines content conventions but not packaging.

## Decision

### Archive format

A `.agentpack` file is a gzipped tarball. Extension: `.agentpack`.

Naming convention: `{name}-{version}.agentpack` when version is known,
`{name}@{sha}.agentpack` when installed from a git ref with no version.

### Content types

The archive contains ONLY these recognized content directories:

| Type     | Directory   | Format                         | Description                         |
| -------- | ----------- | ------------------------------ | ----------------------------------- |
| skills   | `skills/`   | Markdown (SKILL.md in subdirs) | Agent skills                        |
| commands | `commands/` | Markdown                       | Slash commands                      |
| agents   | `agents/`   | Markdown                       | Agent definitions                   |
| hooks    | `hooks/`    | JSON + shell scripts           | Event hooks                         |
| mcp      | `mcp/`      | JSON                           | MCP server configs (remote/ux only) |
| settings | `settings/` | JSON                           | Settings fragments                  |

### Excluded content

These are NEVER included in a package, even if present in the source:

| Excluded                                  | Reason                                 |
| ----------------------------------------- | -------------------------------------- |
| `.git/`                                   | Repository metadata                    |
| `.github/`, `.gitlab/`                    | CI/CD configuration                    |
| `README.md`, `LICENSE`, `CONTRIBUTING.md` | Repository docs                        |
| `CODE_OF_CONDUCT.md`, `SECURITY.md`       | Repository governance                  |
| `CITATION.cff`                            | Citation metadata                      |
| `tools/`, `scripts/` (at root)            | Repository tooling                     |
| `mappings/`, `assets/`, `images/`         | Non-content resources                  |
| `.gitignore`, `.gitattributes`            | Git configuration                      |
| `index.json`                              | Registry indexes                       |
| Binary executables                        | Security policy — no binaries          |
| `node_modules/`, `vendor/`                | Dependency dirs                        |
| `.claude-plugin/`                         | Generated at install by target drivers |

### Metadata

Every package contains `.agentpack/` at the root:

```
.agentpack/
  metadata.yaml         # package identity and provenance
  checksums.txt         # SHA256 of every content file
```

#### metadata.yaml

```yaml
name: my-plugin
version: 1.0.0
source: github.com/org/my-plugin
ref: v1.0.0
sha: abc1234def5678901234567890123456789abcde
built: 2026-05-25T14:30:00Z
builder: agentpack/dev
```

All YAML. No JSON for agentpack's own metadata.

#### checksums.txt

SHA256 of every content file in `sha256sum(1)` format:

```
e3b0c44298fc1c14...  skills/review/SKILL.md
a7ffc6f8bf1ed766...  commands/scan.md
```

### Package manifest (agentpack.yaml)

The optional build manifest declares what to include:

```yaml
name: my-plugin
version: 1.0.0
description: 'My plugin'

skills:
  - skills/**/*
commands:
  - commands/*.md
hooks:
  - hooks/hooks.json
  - hooks/*.sh
mcp:
  - type: remote
    name: my-api
    url: 'https://mcp.example.com/v1'
```

When `agentpack.yaml` is absent (e.g. installing from a third-party repo), the
builder auto-generates one by scanning for standard content directories.

### Build pipeline

Every install goes through the build step:

```
source → clone/fetch → build .agentpack → store → install
```

| Source                         | How the package is built                             |
| ------------------------------ | ---------------------------------------------------- |
| Git repo with `agentpack.yaml` | `build.Run()` using the manifest                     |
| Git repo without manifest      | Auto-detect content dirs → generate manifest → build |
| `.agentpack` file or URL       | Already a package — store and install                |

### Storage

```
~/.config/agentpack/
  cache/          ← go-git clone cache
  archives/       ← built .agentpack files
  packages/       ← installed package manifests (YAML)
```

Archives use the naming: `{name}@{sha}.agentpack` for git sources,
`{name}-{version}.agentpack` for versioned sources.

### Target translation

The package format is generic. Target drivers translate at install time:

| Target      | What it installs                | Where                                                     |
| ----------- | ------------------------------- | --------------------------------------------------------- |
| Claude Code | skills, commands, agents, hooks | `.claude/skills/`, `.claude/commands/`, `.claude/agents/` |
| Cursor      | skills only                     | `.cursor/rules/`                                          |
| Universal   | skills only                     | `.agents/skills/`                                         |

Target drivers read from the package and write to their platform-specific paths.
Content types the target doesn't support are silently skipped.

### MCP configuration

Only `remote` and `ux` MCP types are allowed:

```yaml
mcp:
  - type: remote
    name: my-api
    url: 'https://mcp.example.com/v1'
  - type: ux
    name: my-server
    package: '@mycompany/my-mcp-server'
```

`type: binary` is rejected at build time — no executables in packages.

## Consequences

- Clean packages — no repo cruft, no binaries
- Uniform pipeline — every install path converges to archive
- Offline capable — cached archives enable reinstall without network
- Verifiable — SHA256 checksums for every file
- Safe — no binary execution, inspectable text content
- Extensible — new content types added by extending the directory list
- agentskills.io compatible — skills/ directory convention matches
