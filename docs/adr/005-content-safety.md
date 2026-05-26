# ADR-005: Content Safety and Executable Prompts

## Status

Proposed

## Decision Drivers

- Plugins may contain shell scripts, Python scripts, or other executable content
- Silently installing executable content is a security risk
- Blocking all executable content is too restrictive (hooks need scripts)
- Users should make informed decisions about what they install

## Considered Alternatives

- **Block all executables** — safe but prevents hooks with shell scripts,
  skills with reference scripts, and other legitimate use cases
- **Allow everything silently** — convenient but users don't know what
  they're installing, mirrors the curl-pipe-bash anti-pattern
- **Text-only policy** (ADR-001 original) — ambiguous about scripts vs
  binaries, doesn't match what plugins actually ship

## Context

ADR-001 states "no binary executables" but the archive format allows
shell scripts in `hooks/` and plugins in practice contain Python and
shell scripts (e.g. `scripts/agent.py`, `hooks/lint.sh`). The policy
needs to distinguish between compiled binaries (never allowed) and
text-based scripts (allowed with user consent).

## Decision

### Content classification

| Category | Examples | Policy |
|---|---|---|
| Safe | `.md`, `.json`, `.yaml`, `.txt` | Install without prompt |
| Executable | `.sh`, `.py`, `.js`, `.ts`, `.rb`, `.pl` | Prompt user before install |
| Binary | ELF, Mach-O, `.exe`, `.dll`, `.so`, `.dylib` | Reject always |

### Detection

During `add`, after extracting the archive and before installing:

1. Scan all files in the archive
2. Classify each file by extension and content (check for shebang `#!`)
3. If any executable content is found, prompt the user
4. If any binary content is found, reject with error

### User prompt

```
⚠ Package contains executable content:
  hooks/lint.sh          (shell script)
  scripts/agent.py       (python script)

Allow? Only add packages from sources you trust.

❯ 1. Yes, I trust this package
  2. No, cancel
```

### Skip prompt

- `--trust` flag on `add` skips the prompt (for CI and scripting)
- `install` (from manifest) skips the prompt — the manifest is the
  trust declaration
- Packages containing only safe content never prompt

### Binary detection

Check file content, not just extension — a file named `helper` with
no extension could be an ELF binary. Use magic bytes:

- ELF: `\x7fELF`
- Mach-O: `\xfe\xed\xfa\xce` / `\xfe\xed\xfa\xcf` / `\xca\xfe\xba\xbe`
- PE: `MZ`

## Consequences

- Users are informed about executable content before it touches their system
- Compiled binaries are always rejected — no attack surface
- Scripts are allowed with consent — hooks and reference code work
- CI pipelines use `--trust` to avoid interactive prompts
- `install` from manifest implies trust — declarative config is the
  trust boundary
