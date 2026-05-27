# Usage

## Adding packages

### From a git repo

```bash
agentpack add github.com/org/skills-repo
agentpack add https://github.com/org/skills-repo
agentpack add git@gitlab.com:org/private-repo
```

### Pinning a version

```bash
agentpack add github.com/org/repo#v1.0.0          # tag
agentpack add github.com/org/repo#main             # branch
agentpack add github.com/org/repo#abc1234          # commit SHA
```

### Filtering specific skills

```bash
agentpack add org/repo@review                                  # @skill shorthand
agentpack add github.com/org/repo --skill review --skill deploy # multiple skills
```

### From a local archive

```bash
agentpack add plugin.agentpack
agentpack add ~/Downloads/my-plugin-1.0.0.agentpack
```

### From a URL

```bash
agentpack add https://example.com/plugin.agentpack
```

### Private repos

agentpack reads `~/.netrc` for credentials:

```
machine github.com
login your-username
password ghp_your-token
```

SSH URLs (`git@github.com:org/repo`) use your SSH keys automatically.

## Managing packages

```bash
agentpack list                              # list installed packages
agentpack list --outdated                   # check for available updates
agentpack info my-plugin                    # show package details and files
agentpack info plugin.agentpack             # peek inside an archive
agentpack del my-plugin                     # delete a plugin
```

## Building packages

Create `agentpack.yaml` in your repo root:

```yaml
name: my-plugin
version: 1.0.0
description: 'My skills package'

skills:
  - skills/*.md
commands:
  - commands/*.md
agents:
  - agents/*.md
hooks:
  - hooks/hooks.json
  - hooks/*.sh
mcp:
  - type: remote
    name: my-api
    url: 'https://mcp.example.com/v1'
  - type: ux
    name: my-server
    package: '@mycompany/my-mcp-server'
settings:
  - settings/*.json
```

Then build:

```bash
agentpack build
```

This produces `my-plugin-1.0.0.agentpack` — a checksummed archive ready to
distribute. The SHA256 hash is printed to stdout for publishing alongside the
archive.

### Multi-plugin manifests

For repos with multiple plugins or non-standard layouts:

```yaml
author:
  name: 'Ops Team'
  email: 'ops@example.com'
license: MIT

plugins:
  - name: k8s-helpers
    version: 2.0.0
    description: 'Kubernetes workflow skills'
    skills:
      - src: prompts/kubernetes/*.md
        dest: skills/
    commands:
      - src: cli/k8s/deploy.md
        dest: commands/deploy.md

  - name: terraform-linter
    version: 1.3.0
    description: 'Terraform plan review'
    skills:
      - src: prompts/terraform/*.md
        dest: skills/
```

Build specific plugins:

```bash
agentpack build k8s-helpers
```

## Installing from a manifest

Create `agentpack-packages.yaml` for reproducible installs:

```yaml
packages:
  - name: security-skills
    git: github.com/org/security-skills
    ref: v1.0.0
    skills:
      - code-review
    targets:
      - claude-code

  - name: devops-skills
    git: github.com/org/devops-skills

  - name: offline-plugin
    source: ~/Downloads/plugin.agentpack

  - name: remote-plugin
    source: https://example.com/plugin.agentpack
```

Fields:

- `git` / `source` — where to fetch from (mutually exclusive)
- `ref` — git tag, branch, or SHA to pin (optional)
- `skills` — only install named skills (optional, all if omitted)
- `targets` — only install to named agents (optional, auto-detect if omitted)

Then install:

```bash
agentpack install
```

When `agentpack.lock` exists, locked SHAs are used for reproducibility. Running
`agentpack add` also updates both the yaml and the lockfile.

## Verifying packages

Internal checksums (corruption detection):

```bash
agentpack verify plugin.agentpack
```

External SHA256 (tamper detection):

```bash
agentpack verify plugin.agentpack --sha256 6dddd656a1c5eefb...
```

When the archive is stored in `~/.config/agentpack/archives/`, the `.sha256`
file is auto-detected — no `--sha256` flag needed.

## File locations

```
~/.config/agentpack/
  archives/       built .agentpack files + .sha256 hashes
  packages/       installed package manifests (YAML)
  cache/          go-git clone cache
```

Project-local installs write to the current directory:

```
.claude/skills/           Claude Code
.claude/commands/         Claude Code
.claude/agents/           Claude Code
.windsurf/skills/{name}/  Windsurf
.agents/skills/{name}/    All other agents (Cursor, Copilot, Gemini CLI, Universal, etc.)
```

## Inspecting packages

Peek inside an `.agentpack` archive before adding:

```bash
agentpack info plugin.agentpack
```

Shows package metadata, file listing with sizes, SHA checksums, and integrity
verification.

## Examples

### Manifest examples

How to write `agentpack.yaml` for different project layouts:

- [`manifests/single-plugin/`](../examples/manifests/single-plugin/) — standard
  layout with skills, commands, hooks, agents, MCP, settings
- [`manifests/multi-plugin/`](../examples/manifests/multi-plugin/) — monorepo
  with nonstandard directories, src/dest remapping

### Go library

Using agentpack packages programmatically:

- [`go/install/`](../examples/go/install/) — install a package from a git repo
- [`go/list/`](../examples/go/list/) — list installed packages
