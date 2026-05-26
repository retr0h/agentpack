# ADR-003: Dependency Management Architecture

## Status

Accepted

## Decision Drivers

- Reproducible installs across machines (CI, teammates)
- Project-local scope (no global state)
- Bun-like UX (add updates manifest, install from manifest)
- Existing users expect lockfile semantics from npm/Bun/Cargo

## Considered Alternatives

- **Global lockfile only** — simpler but not project-local, can't be checked
  into git, no reproducibility across machines
- **No lockfile (always resolve fresh)** — simplest but no reproducibility,
  different installs on different machines
- **npm-style node_modules** — heavyweight, not appropriate for config files and
  markdown skills

## Context

agentpack needs a clear model for how packages are tracked, resolved, and
reproduced. The current implementation has overlapping concerns: a registry
(per-package YAML in `~/.config/agentpack/packages/`), a lockfile
(`global.yaml`), and a declarative config (`agentpack-packages.yaml`) — with
unclear roles and boundaries.

Users expect Bun-like behavior: `add` updates a manifest, `install` reproduces
what's in the manifest, and a lockfile pins exact versions.

## Decision

### Source of truth hierarchy

```
agentpack-packages.yaml   ← user-edited spec (what you want)
agentpack.lock            ← resolved snapshot (what you got)
.config/agentpack/packages/  ← runtime registry (what's installed)
```

1. **`agentpack-packages.yaml`** is the spec. Checked into git. Declares
   packages with source and optional ref. Equivalent to `package.json`.

2. **`agentpack.lock`** is the resolved snapshot. Checked into git. Maps each
   package to the exact commit SHA that was resolved at install time. Equivalent
   to `bun.lockb`. Guarantees reproducible installs across machines.

3. **Registry** (`~/.config/agentpack/packages/*.yaml`) is the runtime state —
   what's actually on disk right now. Not checked into git. Equivalent to
   `node_modules`.

### Commands

| Command           | Behavior                                                                            |
| ----------------- | ----------------------------------------------------------------------------------- |
| `add <source>`    | Adds entry to `agentpack-packages.yaml`, installs, updates `agentpack.lock`         |
| `install`         | Installs everything from `agentpack-packages.yaml` using locked SHAs when available |
| `del <name>`      | Removes from `agentpack-packages.yaml`, deletes installed files, updates lock       |
| `list`            | Shows what's installed (from registry)                                              |
| `list --outdated` | Compares locked SHAs against remote HEAD                                            |

`sync` is renamed to `install` to match Bun's vocabulary. `install` with no args
installs the full manifest. This is the Bun model: `bun add` ≈ `agentpack add`,
`bun install` ≈ `agentpack install`.

### Lockfile behavior

When `agentpack.lock` exists:

- `install` uses the locked SHA for each package (reproducible)
- `add` resolves fresh, updates both yaml and lock

When `agentpack.lock` does not exist:

- `install` resolves refs from the yaml (first install on a new machine)
- Creates `agentpack.lock` after resolving

When a package in the yaml has no `ref`:

- Lock pins to whatever HEAD was at resolve time
- `add <source>` again re-resolves and updates the lock

### Remove global.yaml

The current `global.yaml` lockfile is replaced by `agentpack.lock` in the
project directory. There is no global lockfile — agentpack is project-local,
like Bun.

### Config merging (Claude Code)

When a plugin declares MCP servers, hooks, or settings, agentpack must merge
them into the target's config — not just copy files. For Claude Code this means:

- **MCP servers** → merge into `.claude/settings.json` under `mcpServers` key,
  namespaced by plugin name
- **Hooks** → merge into `.claude/settings.json` under `hooks` key
- **Settings** → merge into `.claude/settings.json`, plugin values override
  defaults but don't clobber user settings outside the plugin's namespace

On `del`, the merged entries are removed (the registry tracks what was added
per-plugin so the merge is reversible).

This is a significant implementation effort and may warrant its own ADR for the
merge strategy details.

### `agentpack-packages.yaml` format

```yaml
packages:
  - name: security-skills
    git: github.com/org/security-skills
    ref: v1.0.0

  - name: devops-skills
    git: github.com/org/devops-skills
    # no ref = latest, pinned in lockfile

  - name: offline-plugin
    source: ~/Downloads/plugin.agentpack
```

### `agentpack.lock` format

```yaml
lockVersion: 1
packages:
  - name: security-skills
    source: github.com/org/security-skills
    ref: v1.0.0
    sha: abc1234567890abcdef1234567890abcdef123456
    resolved: '2026-05-25T21:00:00Z'

  - name: devops-skills
    source: github.com/org/devops-skills
    sha: def5678901234abcdef5678901234abcdef567890
    resolved: '2026-05-25T21:00:00Z'
```

## Consequences

- `add` becomes a write operation (modifies yaml + lock + installs)
- `install` with no args is the reproducible install command
- The lockfile enables CI reproducibility (same SHA on every machine)
- `global.yaml` is removed — simplifies the codebase
- Config merging is a new capability that needs careful design
- The `sync` command name is retired in favor of `install`

## Influences

- **Bun**: `add` updates manifest, `install` from manifest, lockfile for
  reproducibility
- **Cargo**: `Cargo.toml` (spec) + `Cargo.lock` (resolved)
- **Go modules**: `go.mod` (spec) + `go.sum` (checksums)
