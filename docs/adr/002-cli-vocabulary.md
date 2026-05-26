# ADR-002: CLI Command Vocabulary

## Status

Accepted

## Context

agentpack needs a consistent, intuitive CLI vocabulary. Early versions
used traditional package manager verbs (`install`, `remove`, `update`,
`show`, `outdated`) drawn from apt and npm. As the tool matured, these
felt misaligned with agentpack's project-local, declarative model.

## Decision

Adopt a vocabulary inspired by APK (Alpine Package Keeper) and Bun,
with adjustments for agentpack's two-persona design.

### Commands

| Command | Description |
|---|---|
| `add` | Add a plugin (re-add = upgrade) |
| `del` | Delete a plugin |
| `list` | List installed (`--outdated` checks for updates) |
| `info` | Show details (installed package or .agentpack archive) |
| `build` | Build .agentpack archives from manifest |
| `sync` | Declarative install from config |
| `verify` | Verify archive checksums |
| `version` | Print version |

### Aliases

| Alias | Command |
|---|---|
| `ls` | `list` |

### Why no `upgrade` command

In APK, `apk add <pkg>` already handles upgrade — re-adding fetches the
latest. A separate `upgrade` command conflicts with declarative config
(`agentpack-packages.yaml`), where packages may be pinned to specific
refs. Upgrading config-managed packages should be done by editing the
config and re-running `sync`, not by a command that circumvents the
declared spec.

### Why `outdated` is a flag, not a command

APK uses `apk list --upgradable`. Checking for updates is a filtered
view of the installed list, not a separate operation. The `--outdated`
flag on `list` keeps the command surface small and discoverable.

## Influences

- **APK**: `add`, `del`, `info`, `list`, `verify` vocabulary
- **Bun**: declarative-first model where config is the upgrade mechanism
- **Helm**: `show` handling both installed and archive targets
