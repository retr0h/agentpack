# Claudia Build System Design

## Problem

Claude Code plugins are distributed via git-based marketplaces. Users who lack
git access (no Tailscale/Aperture, no GitLab credentials) cannot install custom
plugins, skills, MCP servers, or hooks. There is no existing package manager for
distributing Claude Code plugins outside of git.

Claudia solves this by building checksummed tarballs that replicate the exact
directory structure Claude Code expects, so the end user can unpack them into
`~/.claude/plugins/` and they just work.

## Scope

This spec covers **Phase 1: the build system only** (`claudia build` and
`claudia verify`). Installation (`claudia install`) is a future phase.

## Manifest: `claudia.yaml`

The build manifest lives at the root of the plugin repo. It declares one or
more plugins to build from the repo's contents.

### Single-plugin manifest

```yaml
name: my-plugin
version: 1.0.0
description: "My Claude Code plugin"

author:
  name: "John Dewey"
  email: "john@dewey.ws"

license: MIT
homepage: "https://github.com/retr0h/my-plugin"
keywords: [notifications, hooks]
category: productivity

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
    name: my-server
    src: bin/my-server
    platforms: [darwin-arm64, darwin-amd64, linux-amd64]
    args: ["--port", "3000"]

  - type: remote
    name: my-remote
    url: "https://mcp.example.com/v1"

  - type: ux
    name: my-ux-server
    package: "@mycompany/my-mcp-server"

binaries:
  - bin/my-tool

settings:
  - settings.json
```

### Multi-plugin manifest

For repos that contain multiple plugins (like an aikit monorepo with skills,
agents, and commands scattered across non-standard directories):

```yaml
author:
  name: "John Dewey"
  email: "john@dewey.ws"
license: MIT

plugins:
  - name: zeek-pros
    version: 1.0.0
    description: "Zeek protocol analysis skills"
    category: security
    skills:
      - src: prompts/zeek/*.md
        dest: skills/

  - name: git-commands
    version: 2.1.0
    description: "Git workflow commands and hooks"
    category: development
    commands:
      - src: cli/git/*.md
        dest: commands/
    hooks:
      - src: hooks/git-hooks.json
        dest: hooks/hooks.json
      - src: scripts/git/*.sh
        dest: hooks/

  - name: my-mcp-server
    version: 0.5.0
    description: "Custom MCP server"
    mcp:
      - type: binary
        src: bin/my-server
        platforms: [darwin-arm64, linux-amd64]
    settings:
      - src: config/mcp-settings.json
        dest: settings/settings.json
```

Shared fields (`author`, `license`) cascade down to all plugins. Each plugin
can override any shared field.

### Content entry formats

Every content field (skills, commands, hooks, agents, binaries, settings)
accepts two forms:

**Simple glob** -- files are already in the right place, paths are preserved:

```yaml
skills:
  - skills/*.md
```

**Source-to-destination mapping** -- files need remapping into the plugin
structure:

```yaml
skills:
  - src: prompts/zeek/*.md
    dest: skills/
```

When `dest` ends with `/`, the source filename is preserved. When `dest` is a
full path (e.g. `skills/review.md`), the file is renamed.

### Field reference

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Plugin name (kebab-case) |
| `version` | yes | SemVer version string |
| `description` | yes | One-line description |
| `author` | no | Name and email |
| `license` | no | SPDX license identifier |
| `homepage` | no | Project URL |
| `keywords` | no | Search/discovery tags |
| `category` | no | Claude Code plugin category |
| `skills` | no | Skill markdown files (glob or src/dest) |
| `commands` | no | Command markdown files (glob or src/dest) |
| `hooks` | no | hooks.json and hook scripts (glob or src/dest) |
| `agents` | no | Agent markdown files (glob or src/dest) |
| `mcp` | no | MCP server entries (see below) |
| `binaries` | no | Pre-built executables (glob or src/dest) |
| `settings` | no | JSON fragments for settings.json (glob or src/dest) |

### MCP entry types

Each MCP entry type causes claudia to **generate** the `.mcp.json` that
Claude Code needs to launch the server. You do not write this file yourself
-- claudia creates it from the manifest fields. This is critical because the
developer's `.mcp.json` (e.g., `go run github.com/retr0h/foo`) won't work
on a system without Go -- the packaged version must reference the pre-built
binary.

**binary** -- A pre-built MCP server executable. You cross-compile the
binary yourself (via `go build`, goreleaser, CI, etc.) and point `src` at
the result. Claudia packages the binary and generates a `.mcp.json` that
launches it using `${CLAUDE_PLUGIN_ROOT}` path substitution -- the same
mechanism Claude Code uses natively (e.g., `claude-notifications-go`).

```yaml
mcp:
  - type: binary
    src: bin/my-server-darwin-arm64
    name: my-server
    platforms: [darwin-arm64]
    args: ["--port", "3000"]
    env:
      MY_VAR: "value"
```

Generated `.mcp.json`:
```json
{
  "mcpServers": {
    "my-server": {
      "command": "${CLAUDE_PLUGIN_ROOT}/mcp/my-server-darwin-arm64",
      "args": ["--port", "3000"],
      "env": {
        "MY_VAR": "value"
      }
    }
  }
}
```

**remote** -- An MCP server hosted at a remote endpoint. No binary needed.
Claudia generates a `.mcp.json` with the URL.

```yaml
mcp:
  - type: remote
    name: my-remote-server
    url: "https://mcp.example.com/v1"
```

Generated `.mcp.json`:
```json
{
  "mcpServers": {
    "my-remote-server": {
      "url": "https://mcp.example.com/v1"
    }
  }
}
```

Alternatively, if you already have a hand-written `.mcp.json`, use the
`config` field to include it as-is:

```yaml
mcp:
  - type: remote
    config: .mcp.json
```

**ux** -- A config referencing a UX/npx package. The end user must have
the package runner installed, but no binary ships in the archive.

```yaml
mcp:
  - type: ux
    name: my-ux-server
    package: "@mycompany/my-mcp-server"
    args: ["--verbose"]
```

Generated `.mcp.json`:
```json
{
  "mcpServers": {
    "my-ux-server": {
      "command": "npx",
      "args": ["@mycompany/my-mcp-server", "--verbose"]
    }
  }
}

## Archive format

A `.claudia` file is a gzipped tarball (`.tar.gz` with a `.claudia` extension).
The internal layout mirrors `~/.claude/plugins/` so the end user can install
with a plain `tar xzf`:

```bash
tar xzf zeek-pros-1.0.0.claudia -C ~/.claude/plugins/
```

### Internal layout

```
zeek-pros-1.0.0.claudia
  marketplaces/zeek-pros/
    .claude-plugin/
      marketplace.json     # Generated marketplace descriptor
      plugin.json          # Generated plugin descriptor
    .claudia/
      metadata.json        # Build metadata (git SHA, version, timestamps)
      checksums.txt        # Per-file SHA256 checksums
      claudia.yaml         # Copy of the source manifest (this plugin only)
    skills/
      *.md
    commands/
      *.md
    hooks/
      hooks.json
      *.sh
    agents/
      *.md
    mcp/
      bin/my-server        # Binary MCP servers
      .mcp.json            # Remote MCP configs
      ux.json              # UX/npx MCP configs
    bin/
      my-tool              # Pre-built binaries
    settings/
      settings.json        # Settings fragments
```

The metadata directory (`.claudia/`) lives inside the marketplace directory,
not at the top level. This means:

- **No collision** -- each plugin's metadata is namespaced inside its own
  `marketplaces/{name}/` directory. Untarring ten plugins never stomps.
- **Self-contained** -- delete `marketplaces/zeek-pros/` and the metadata
  goes with it. No orphaned state.
- **Follows Claude Code convention** -- Claude Code puts `.claude-plugin/`
  inside the marketplace directory. `.claudia/` is the same pattern. Claude
  Code ignores it because it only looks for `.claude-plugin/`.

### `.claudia/metadata.json`

Generated at build time by capturing state from the git repo:

```json
{
  "name": "zeek-pros",
  "version": "1.0.0",
  "gitCommitSha": "a1b2c3d4e5f6...",
  "gitBranch": "main",
  "buildTimestamp": "2026-05-22T14:30:00Z",
  "builderVersion": "0.1.0",
  "platform": "darwin-arm64"
}
```

The `gitCommitSha` is used by the install side (future phase) to populate
`installed_plugins.json` entries, matching how Claude Code tracks plugin
versions natively.

### `.claudia/checksums.txt`

Every file in the archive (except `checksums.txt` itself) gets a SHA256
checksum. Paths are relative to the archive root. Format matches `sha256sum`
output:

```
e3b0c44298fc1c14...  marketplaces/zeek-pros/.claude-plugin/marketplace.json
a7ffc6f8bf1ed766...  marketplaces/zeek-pros/.claude-plugin/plugin.json
b94d27b9934d3e08...  marketplaces/zeek-pros/.claudia/metadata.json
f2ca1bb6c7e907d0...  marketplaces/zeek-pros/.claudia/claudia.yaml
d7a8fbb307d7809e...  marketplaces/zeek-pros/skills/review.md
...
```

### `.claude-plugin/marketplace.json`

Generated from the manifest to match Claude Code's expected format. Follows
the single-plugin marketplace pattern (like `claude-notifications-go`) where
the marketplace and the plugin are one and the same:

```json
{
  "name": "zeek-pros",
  "owner": {
    "name": "John Dewey",
    "email": "john@dewey.ws"
  },
  "metadata": {
    "description": "Zeek protocol analysis skills",
    "version": "1.0.0"
  },
  "plugins": [
    {
      "name": "zeek-pros",
      "source": "./",
      "description": "Zeek protocol analysis skills",
      "version": "1.0.0",
      "author": {
        "name": "John Dewey",
        "email": "john@dewey.ws"
      },
      "license": "MIT",
      "keywords": ["zeek", "protocol", "security"],
      "category": "security"
    }
  ]
}
```

### `.claude-plugin/plugin.json`

Generated from the manifest to match Claude Code's expected format:

```json
{
  "name": "zeek-pros",
  "version": "1.0.0",
  "description": "Zeek protocol analysis skills",
  "author": {
    "name": "John Dewey",
    "email": "john@dewey.ws"
  },
  "license": "MIT",
  "keywords": ["zeek", "protocol", "security"],
  "commands": [
    "./commands/init.md",
    "./commands/analyze.md"
  ]
}
```

The `commands` array is populated from resolved command entries. Skills are
discovered by Claude Code via the directory structure, not plugin.json.

## Build process

`claudia build` performs these steps in order:

1. **Parse manifest** -- Read and validate `claudia.yaml` from the current
   directory. Detect single-plugin vs multi-plugin format.

2. **Select plugins** -- If plugin names are given as arguments, build only
   those. Otherwise build all plugins in the manifest.

3. **Capture metadata** -- Run `git rev-parse HEAD` for SHA, `git
   rev-parse --abbrev-ref HEAD` for branch, capture current timestamp and
   claudia version. Fail if not in a git repo (metadata depends on git state).

4. **For each plugin:**

   a. **Resolve entries** -- Expand globs and resolve src/dest mappings.
      Fail if a declared entry matches zero files (a declared entry implies
      the user expects files to exist). Sections omitted from the manifest
      are simply skipped.

   b. **Validate files** -- Confirm all referenced files exist. For binary
      MCP servers, confirm the binary exists at the `src` path. For remote
      MCP, confirm the `.mcp.json` exists.

   c. **Generate plugin descriptors** -- Create `marketplace.json` and
      `plugin.json` from the manifest fields, inheriting shared fields.

   d. **Compute checksums** -- SHA256 every file that will go into the
      archive. Write `checksums.txt`.

   e. **Write metadata** -- Serialize `metadata.json`.

   f. **Create tarball** -- Pack everything into `{name}-{version}.claudia`
      (a `.tar.gz`), rooted at `marketplaces/{name}/`.

   g. **Print summary** -- Show archive details.

### CLI usage

```bash
claudia build                              # build all plugins
claudia build zeek-pros                    # build one plugin
claudia build zeek-pros git-commands       # build specific plugins
```

### Output

```
$ claudia build
claudia: building zeek-pros v1.0.0 (a1b2c3d)

  skills/       3 files
  commands/     2 files

  checksummed 7 files (SHA256)

  zeek-pros-1.0.0.claudia  (12 KB)
  sha256: e3b0c44298fc1c149afbf4c8996fb924...

claudia: building git-commands v2.1.0 (a1b2c3d)

  commands/     4 files
  hooks/        3 files

  checksummed 9 files (SHA256)

  git-commands-2.1.0.claudia  (8 KB)
  sha256: d7a8fbb307d7809469ca9abcb0082e4f...

2 archives built
```

## Verify process

`claudia verify <archive>` performs:

1. **Extract** the archive to a temp directory.
2. **Locate** `.claudia/checksums.txt` inside the marketplace directory.
3. **Hash** every file listed and compare against expected checksums.
4. **Report** pass/fail per file, exit non-zero on any mismatch.

### CLI usage

```bash
claudia verify zeek-pros-1.0.0.claudia
```

### Output

```
$ claudia verify zeek-pros-1.0.0.claudia
claudia: verifying zeek-pros-1.0.0.claudia

  marketplaces/zeek-pros/.claudia/metadata.json     OK
  marketplaces/zeek-pros/.claudia/claudia.yaml      OK
  marketplaces/zeek-pros/.claude-plugin/marketplace.json  OK
  marketplaces/zeek-pros/.claude-plugin/plugin.json  OK
  marketplaces/zeek-pros/skills/review.md            OK
  marketplaces/zeek-pros/commands/init.md            OK

  6/6 files verified
```

## Architecture

### Package layout

```
internal/manifest/    Parse + validate claudia.yaml (single and multi-plugin)
internal/metadata/    Capture git SHA, branch, timestamp
internal/checksum/    SHA256 per file, write/read checksums.txt
internal/archive/     Create .tar.gz, extract to temp dir
internal/plugin/      Generate marketplace.json + plugin.json
cmd/build.go          Orchestrate build pipeline (per-plugin loop)
cmd/verify.go         Orchestrate verify pipeline
```

### Key types

```go
// internal/manifest

type Manifest struct {
    // Shared fields (cascade to all plugins)
    Author   Author   `yaml:"author"`
    License  string   `yaml:"license"`
    Homepage string   `yaml:"homepage"`

    // Single-plugin mode (name at top level)
    Name        string     `yaml:"name"`
    Version     string     `yaml:"version"`
    Description string     `yaml:"description"`
    Keywords    []string   `yaml:"keywords"`
    Category    string     `yaml:"category"`
    Skills      []Entry    `yaml:"skills"`
    Commands    []Entry    `yaml:"commands"`
    Hooks       []Entry    `yaml:"hooks"`
    Agents      []Entry    `yaml:"agents"`
    MCP         []MCPEntry `yaml:"mcp"`
    Binaries    []Entry    `yaml:"binaries"`
    Settings    []Entry    `yaml:"settings"`

    // Multi-plugin mode
    Plugins []Plugin `yaml:"plugins"`
}

type Plugin struct {
    Name        string     `yaml:"name"`
    Version     string     `yaml:"version"`
    Description string     `yaml:"description"`
    Author      Author     `yaml:"author"`
    License     string     `yaml:"license"`
    Homepage    string     `yaml:"homepage"`
    Keywords    []string   `yaml:"keywords"`
    Category    string     `yaml:"category"`
    Skills      []Entry    `yaml:"skills"`
    Commands    []Entry    `yaml:"commands"`
    Hooks       []Entry    `yaml:"hooks"`
    Agents      []Entry    `yaml:"agents"`
    MCP         []MCPEntry `yaml:"mcp"`
    Binaries    []Entry    `yaml:"binaries"`
    Settings    []Entry    `yaml:"settings"`
}

// Entry supports both simple globs and src/dest mappings.
// YAML unmarshaling: a bare string becomes Entry{Glob: "..."}.
// An object with src/dest becomes Entry{Src: "...", Dest: "..."}.
type Entry struct {
    Glob string `yaml:"-"`
    Src  string `yaml:"src"`
    Dest string `yaml:"dest"`
}

type Author struct {
    Name  string `yaml:"name"`
    Email string `yaml:"email"`
}

type MCPEntry struct {
    Type      string            `yaml:"type"`
    Name      string            `yaml:"name"`
    Src       string            `yaml:"src"`
    Config    string            `yaml:"config"`
    URL       string            `yaml:"url"`
    Package   string            `yaml:"package"`
    Args      []string          `yaml:"args"`
    Env       map[string]string `yaml:"env"`
    Platforms []string          `yaml:"platforms"`
}
```

The `Entry` type requires a custom `UnmarshalYAML` implementation to handle
both string and object forms:

```yaml
# String form → Entry{Glob: "skills/*.md"}
skills:
  - skills/*.md

# Object form → Entry{Src: "prompts/*.md", Dest: "skills/"}
skills:
  - src: prompts/*.md
    dest: skills/
```

```go
// internal/metadata

type Metadata struct {
    Name           string `json:"name"`
    Version        string `json:"version"`
    GitCommitSha   string `json:"gitCommitSha"`
    GitBranch      string `json:"gitBranch"`
    BuildTimestamp string `json:"buildTimestamp"`
    BuilderVersion string `json:"builderVersion"`
    Platform       string `json:"platform"`
}
```

```go
// internal/checksum

type Entry struct {
    Hash string
    Path string
}
```

### Manifest normalization

When a single-plugin manifest is parsed (has `name` at top level, no
`plugins` array), it is normalized into a one-element `Plugins` slice.
All downstream code works with `[]Plugin` regardless of manifest format.

### Dependencies

- `gopkg.in/yaml.v3` -- manifest parsing
- `crypto/sha256` -- checksumming (stdlib)
- `archive/tar` + `compress/gzip` -- tarball creation/extraction (stdlib)
- `os/exec` -- git commands for metadata capture
- `path/filepath` -- glob resolution (stdlib)
- `github.com/spf13/cobra` -- CLI framework

No external dependencies beyond cobra and yaml.v3.

## Error handling

- Missing `claudia.yaml`: `"claudia.yaml not found in current directory"`
- Not a git repo: `"not a git repository; claudia build requires git metadata"`
- Empty manifest: `"no plugins defined in claudia.yaml"`
- Unknown plugin name: `"plugin 'foo' not found in claudia.yaml"`
- Glob matches zero files: `"pattern 'skills/*.md' matched no files"`
- Referenced file missing: `"file not found: bin/my-server"`
- Binary not executable: `"bin/my-server is not executable"`
- Invalid MCP type: `"unknown mcp type 'foo'; expected binary, remote, or ux"`
- Invalid YAML: report parse error with line number
- Archive write failure: wrap OS error with context
- Both formats used: `"manifest has both top-level 'name' and 'plugins'; use one or the other"`

## Testing strategy

Table-driven tests for every public function. Key test scenarios:

- **manifest**: valid single-plugin YAML, valid multi-plugin YAML, missing
  required fields, invalid MCP types, field inheritance from shared to plugin,
  shared field override, string entry vs src/dest entry, both formats used
  simultaneously (error), empty plugins list
- **manifest Entry unmarshaling**: bare string, object with src/dest, object
  with dest ending in `/`, mixed entries in same list
- **metadata**: git repo present, git repo absent, detached HEAD
- **checksum**: single file, multiple files, empty file, binary file,
  verify pass, verify mismatch, missing file in checksums.txt
- **archive**: create + extract round-trip, verify contents match, large
  files, symlinks rejected, path traversal rejected, correct
  `marketplaces/{name}/` prefix in tarball
- **plugin**: marketplace.json generation with all fields, minimal fields,
  plugin.json with commands, without commands, field inheritance

## End-user installation (Phase 1)

Until `claudia install` ships, the end user installs with plain `tar`:

```bash
tar xzf zeek-pros-1.0.0.claudia -C ~/.claude/plugins/
```

First time only, the user needs to enable the plugin in
`~/.claude/settings.json`:

```json
{
  "enabledPlugins": {
    "zeek-pros@zeek-pros": true
  }
}
```

Subsequent updates are just re-running the same `tar` command. The
`enabledPlugins` entry does not change between versions.

## Future phases

- **Phase 2: `claudia install` + `claudia list`** -- unpack archive into
  `~/.claude/plugins/`, update `installed_plugins.json` and `settings.json`
  automatically. `claudia list` shows installed packages with styled output.
- **Phase 3: `claudia-packages.yaml` declarative installs** -- a lockfile
  declaring what should be installed, enabling `claudia sync` to install or
  update everything in one shot:
  ```yaml
  packages:
    - name: zeek-pros
      source: ~/Downloads/zeek-pros-1.0.0.claudia
    - name: git-commands
      source: https://drive.google.com/...
    - name: my-mcp-server
      source: ./my-mcp-server-0.5.0.claudia
  ```
- **Phase 4: pluggable backends** -- fetch archives from Google Drive, S3,
  URLs directly in `claudia install` and `claudia sync`
- **Phase 5: `claudia update`** -- re-run install with version comparison
