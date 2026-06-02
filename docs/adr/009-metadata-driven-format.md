---
status: accepted
date: 2026-06-01
---

# ADR-009: Metadata-Driven Package Format

## Context and Problem Statement

The `.agentpack` archive format (ADR-001) uses directory names as the type
system: files under `skills/` are skills, files under `commands/` are commands,
and so on. Target drivers infer content types by walking these directories.

This couples the archive format to Claude Code's directory conventions. Every
target driver must know that `skills/` means skills and `commands/` means
commands. The format claims to be agent-agnostic, but its internal structure
mirrors a single agent's layout.

As more agents adopt different content types (Cursor supports skills but not
commands, Cline supports hooks, Codex is adding MCP), the format needs a generic
type system that lets packages declare what they contain and lets drivers
declare what they support.

## Decision Drivers

- Format must be truly agent-agnostic — no directory conventions as the type
  contract
- Drivers must be able to declare capabilities and receive only content they
  support
- Existing source repos (agentskills.io convention) must not require changes
- Existing archives should degrade gracefully (backward compatibility)
- Integrity model must follow established package manager patterns (npm, Go)
- Format should be suitable as an industry standard that could be adopted by
  tools like npx skills

## Considered Options

1. **Metadata overlay on current layout** — keep directory structure, add typed
   entries to metadata. Drivers read metadata instead of inferring from
   directories.
2. **Entries-based layout** — restructure archive into `entries/{name}/`
   directories. Metadata is the only way to know types. Requires build pipeline
   translation.
3. **Content-addressed blob store** — files stored as `blobs/{hash}`, metadata
   maps hashes to names and types. Full indirection.

## Decision Outcome

**Option 1: Metadata overlay on current layout.**

The metadata is the real contract between packages and drivers. The directory
layout inside the archive is just storage — an implementation detail of the
build pipeline. Restructuring directories (Option 2) or content-addressing
(Option 3) adds complexity without functional benefit. Drivers that read
metadata are decoupled from the archive layout regardless of what that layout
looks like.

### Content types

Six generic content types form the agentpack format vocabulary. These describe
the _function_ of the content, not what any specific agent calls it. Drivers
translate types to their platform-specific concepts (e.g., a "skill" becomes a
"rule" in Cursor, an "instruction" in Codex).

| Type      | Description                        | Example agents that support it       |
| --------- | ---------------------------------- | ------------------------------------ |
| `skill`   | Knowledge/capability module        | All (Claude Code, Cursor, Codex ...) |
| `command` | User-invoked action                | Claude Code, Cline                   |
| `hook`    | Event-driven automation            | Claude Code, Cline, Codex            |
| `agent`   | Subagent/persona definition        | Claude Code, Cline                   |
| `mcp`     | External service integration (MCP) | All (protocol standard)              |
| `config`  | Configuration the package needs    | Claude Code, Codex, Amp              |

The `setting` type from ADR-001 is renamed to `config` for generality. Types are
intentionally agent-agnostic — if Codex calls skills "instructions", the Codex
driver handles that translation. The type vocabulary is the intermediate
representation.

### metadata.yaml

Every archive contains `.agentpack/metadata.yaml` (replacing the previous
`metadata.json`). This is the single source of truth for package identity and
content.

```yaml
name: jeffallan/claude-skills
version: 1.0.0
sha: e8be415abc1234567890
built: 2026-06-01T00:00:00Z
builder: agentpack/1.2.0
platform: darwin-arm64

entries:
  - name: kubernetes-specialist
    type: skill
  - name: react-expert
    type: skill
  - name: scan
    type: command
  - name: post-write
    type: hook
  - name: my-api
    type: mcp
  - name: theme
    type: config
```

Entry `name` maps to a directory path inside the archive via the build
pipeline's directory conventions (e.g., `skills/kubernetes-specialist/`).
Drivers never need to know this mapping — they receive entries with resolved
paths from the install pipeline.

### checksums.txt removal

The per-file `checksums.txt` inside the archive is removed. It was redundant:

- **Git installs**: the git commit SHA (already in the lockfile) is the
  integrity anchor. The archive is built locally from the cloned content — a
  checksum of the archive would just be checking our own homework.
- **Archive installs**: a `.sha256` sidecar file (produced by `agentpack build`)
  serves as the integrity anchor. The installer verifies the archive hash
  against the sidecar before extraction.

This follows established patterns: npm stores integrity hashes in
`package-lock.json`, Go stores them in `go.sum`. Neither embeds per-file
checksums inside the package itself.

Per-file hashes for installed files (used by the remove pipeline to detect
user-modified files before deletion) come from the driver's `Install()` return
value and are stored in the registry manifest. These are unaffected.

### Integrity model

| Install source    | Integrity anchor                   | Verified when                               |
| ----------------- | ---------------------------------- | ------------------------------------------- |
| Git repo          | Git commit SHA in `agentpack.lock` | `agentpack install` re-resolves to same SHA |
| Pre-built archive | `.sha256` sidecar file             | Before extraction                           |
| Installed files   | Per-file SHA256 in registry        | `agentpack del` checks before deleting      |

### Driver capability model

Each target driver declares which content types it supports via a new
`SupportedTypes()` method on the `Target` interface:

```go
type Target interface {
    Name() string
    DisplayName() string
    SupportedTypes() []string
    Install(ctx context.Context, opts InstallOpts) ([]InstalledFile, error)
}
```

The install pipeline reads metadata entries and filters them by the driver's
supported types before calling `Install()`. Drivers receive only entries they
can handle.

Current driver capabilities:

| Driver      | Supported types                          |
| ----------- | ---------------------------------------- |
| Claude Code | skill, command, hook, agent, mcp, config |
| Cursor      | skill                                    |
| Codex       | skill                                    |
| Copilot     | skill                                    |
| Universal   | skill                                    |
| Windsurf    | skill                                    |

When a driver adds support for a new type (e.g., Cursor adds command support),
existing packages with commands automatically start being installed to Cursor.
No package format change required.

### Install pipeline changes

The `InstallOpts` passed to drivers gains an `Entries` field:

```go
type ContentEntry struct {
    Name string   // e.g. "kubernetes-specialist"
    Type string   // e.g. "skill"
    Root string   // path within extracted archive
}

type InstallOpts struct {
    Entries []ContentEntry
    // ... existing fields unchanged ...
}
```

Pipeline flow:

1. Extract archive
2. Read `.agentpack/metadata.yaml`
3. For each target driver: a. Filter entries to types the driver supports b.
   Pass filtered entries to `driver.Install()` c. Driver iterates entries,
   installs each one to its platform paths

Backward compatibility: if metadata.yaml has no `entries` section (old-format
archive), the pipeline falls back to the current directory-walking behavior.

### Build pipeline changes

- `metadata.yaml` (YAML) replaces `metadata.json` (JSON)
- Build pipeline scans source repo conventional directories (`skills/`,
  `commands/`, etc.), maps each item to a typed entry, writes the entries list
  into metadata.yaml
- `checksums.txt` is no longer generated
- `agentpack build` produces a `.sha256` sidecar alongside the archive

Source repos keep their current layout (`skills/`, `commands/`, etc.). The build
pipeline translates directory conventions into typed entries. Package authors
change nothing.

### Archive layout

The directory structure inside the archive is unchanged from ADR-001. Files
remain in `skills/`, `commands/`, `hooks/`, `agents/`, `mcp/`, `settings/`
directories. This layout is a build-time convention, not a contract — drivers
consume metadata, not directory names.

### What does not change

- Source repo layout
- CLI UX (add, del, ls, info, search)
- Registry format (per-file hashes from drivers for safe removal)
- agentpack-packages.yaml format
- agentpack.lock format (stays v2, git SHA already present)
- Archive container format (gzipped tarball, .agentpack extension)
- @skill filtering (filters entries by name)
- Target auto-detection

## Consequences

- The archive format is decoupled from any agent's directory conventions
- Drivers explicitly declare capabilities — no silent assumptions about what
  content types exist
- New content types can be added by extending the type vocabulary without
  changing the archive layout
- Existing packages work via backward-compatible fallback
- The format is suitable as an industry standard — types describe function, not
  agent-specific concepts
- The integrity model follows npm/Go patterns rather than inventing a custom
  per-file checksum scheme

## More Information

- Supersedes [ADR-001](001-agentpack-format.md) (archive format and content
  types)
- Related to [ADR-004](004-config-merging.md) (`setting` type renamed to
  `config`)
- Related to [ADR-005](005-content-safety.md) (content classification moves from
  checksums.txt-adjacent to metadata.yaml)
- Related to [ADR-008](008-content-filtering.md) (@skill filtering now operates
  on metadata entries)
