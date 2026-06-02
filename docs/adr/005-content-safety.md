---
status: accepted
date: 2026-05-25
---

# ADR-005: Content Safety and Executable Prompts

## Context and Problem Statement

ADR-001 states "no binary executables" but the archive format allows shell
scripts in `hooks/` and plugins in practice contain Python and shell scripts
(e.g. `scripts/agent.py`, `hooks/lint.sh`). The policy needs to distinguish
between compiled binaries (never allowed) and text-based scripts (allowed with
user consent).

## Decision Drivers

- Plugins may contain shell scripts, Python scripts, or other executable content
- Silently installing executable content is a security risk
- Blocking all executable content is too restrictive (hooks need scripts)
- Users should make informed decisions about what they install
- Safety classification should be a first-class package property, not a runtime
  scan

## Considered Options

- **Block all executables** — safe but prevents hooks with shell scripts, skills
  with reference scripts, and other legitimate use cases
- **Allow everything silently** — convenient but users don't know what they're
  installing, mirrors the curl-pipe-bash anti-pattern
- **Text-only policy** (ADR-001 original) — ambiguous about scripts vs binaries,
  doesn't match what plugins actually ship
- **Scan at install time** — works but duplicates effort on every install,
  classification isn't tracked or checksummed, can't be inspected before
  installing

## Decision Outcome

### Content classification is a build-time property

Classification happens at **build time** and is embedded in the package metadata
(`.agentpack/metadata.json`). It is checksummed alongside all other content. No
scanning happens at install time — the installer reads the classification from
trusted, verified metadata.

### Classification categories

| Category   | Extensions                                       | Detection                     | Policy                     |
| ---------- | ------------------------------------------------ | ----------------------------- | -------------------------- |
| Safe       | `.md`, `.json`, `.yaml`, `.yml`, `.txt`, `.toml` | Extension                     | Install without prompt     |
| Executable | `.sh`, `.py`, `.js`, `.ts`, `.rb`, `.pl`, `.lua` | Extension + shebang `#!`      | Prompt user before install |
| Binary     | —                                                | Magic bytes (ELF, Mach-O, PE) | Reject at build time       |

### Metadata format

The `content` field is added to `.agentpack/metadata.json`:

```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "gitCommitSHA": "abc1234",
  "buildTimestamp": "2026-05-26T...",
  "content": {
    "safe": ["skills/review/SKILL.md", "commands/scan.md", "mcp/my-api.json"],
    "executable": ["hooks/lint.sh", "scripts/agent.py"]
  }
}
```

Binary files are rejected at build time — they never appear in metadata because
the build fails.

### Build-time behavior

1. Classify every content file by extension and magic bytes
2. If any binary is detected, fail the build with an error
3. Embed the classification in `metadata.json`
4. Classification is covered by `checksums.txt` — tamper-proof

### Install-time behavior

1. Read `content` from verified `metadata.json`
2. If `executable` list is non-empty, prompt the user:

```
⚠ Package contains executable content:
  hooks/lint.sh          (shell script)
  scripts/agent.py       (python script)

Allow? Only add packages from sources you trust.

❯ 1. Yes, I trust this package
  2. No, cancel
```

3. If user declines, abort the install

### Info command

`agentpack info plugin.agentpack` shows the classification:

```
Package:   my-plugin
Version:   v1.0.0
Built:     2026-05-26
SHA:       abc1234
Content:   3 safe, 2 executable
```

### Skip prompt

- `--trust` flag on `add` skips the prompt (for CI and scripting)
- `install` (from manifest) skips the prompt — the manifest is the trust
  declaration
- Packages containing only safe content never prompt

### Binary detection

Check file content, not just extension — a file named `helper` with no extension
could be an ELF binary. Magic bytes:

- ELF: `\x7fELF`
- Mach-O: `\xfe\xed\xfa\xce` / `\xfe\xed\xfa\xcf` / `\xca\xfe\xba\xbe`
- PE: `MZ`

## Consequences

- Safety is a first-class package property — inspectable, checksummed, tracked
- Build fails fast on binaries — they never enter the ecosystem
- Users are informed about executable content before it touches their system
- `info` shows content classification before you even add a package
- CI pipelines use `--trust` to avoid interactive prompts
- `install` from manifest implies trust — declarative config is the trust
  boundary
- No runtime scanning overhead — classification is pre-computed
