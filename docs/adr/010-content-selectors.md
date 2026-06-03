---
status: accepted
date: 2026-06-02
---

# ADR-010: Content Selectors and Version Pinning Syntax

## Context and Problem Statement

The CLI syntax for selecting content from a package has two problems:

1. **`@` means skill, not version.** Every other package manager (npm, cargo,
   go, pip) uses `@` for version pinning. agentpack uses it for skill selection
   (`owner/repo@skill-name`). This violates user expectations and creates
   ambiguity — `owner/repo@v1.2.3` could be a version or a skill named "v1.2.3".
   The same design flaw in npx skills caused a supply-chain security issue
   ([vercel-labs/skills#863](https://github.com/vercel-labs/skills/issues/863)).

2. **Only skills can be selected.** The `@skill` syntax and `--skill` flag only
   filter skills. There is no way to select a specific command, hook, MCP
   config, or other content type from a package. With six content types
   (ADR-009), the selector should work with all of them.

## Decision Drivers

- Follow universal package manager convention for version pinning
- Leverage the typed content model from ADR-009
- Eliminate the `--skill` flag and `#ref` fragment syntax
- Support selecting any content type, not just skills
- Keep the CLI syntax intuitive and unambiguous

## Considered Options

1. **`@` for version, `:` for content selection** — follows npm/cargo/go for
   versions, uses `:` (git path convention) for content. Unified `type/name`
   selector works with all content types.
2. **Keep `@` for skills, add `--version` flag** — backward compatible but
   leaves the convention mismatch and doesn't solve the content selection gap.
3. **`@` for version, `--skill`/`--command`/`--hook` flags** — fixes version
   syntax but adds N flags instead of one unified selector.

## Decision Outcome

**Option 1: `@` for version, `:` for content selection.**

### CLI syntax

```
agentpack add owner/repo                          # latest, all content
agentpack add owner/repo:skill/k8s                # latest, one skill
agentpack add owner/repo:command/scan             # latest, one command
agentpack add owner/repo:hook/on-save             # latest, one hook
agentpack add owner/repo:mcp/my-api              # latest, one MCP config
agentpack add owner/repo@v2.0.0                   # pinned version, all content
agentpack add owner/repo@v2.0.0:skill/k8s        # pinned version, one skill
agentpack add owner/repo:skill/k8s:command/scan   # multiple items
```

The `:` separator introduces a content selector in the form `type/name`. The
type maps directly to the six content types defined in ADR-009 (`skill`,
`command`, `hook`, `agent`, `mcp`, `config`). The name is the entry name from
the package metadata.

Multiple selectors can be chained with additional `:` separators.

### Version pinning with `@`

The `@` separator specifies a git ref (tag, branch, or SHA):

```
owner/repo@v2.0.0          # tag
owner/repo@main            # branch
owner/repo@abc1234         # commit SHA
```

This replaces the previous `#ref` fragment syntax. The resolved ref is recorded
in `agentpack.lock` for reproducible installs.

### Manifest syntax

The `agentpack-packages.yaml` spec file gains a `content` field that replaces
the previous `skills` field:

```yaml
packages:
  - name: owner/repo
    git: github.com/owner/repo
    ref: v2.0.0
    content:
      - skill/kubernetes-specialist
      - skill/react-expert
      - command/scan
      - mcp/my-api
```

When `content` is omitted, all content from the package is installed. When
present, only the listed items are installed. Each entry uses the same
`type/name` format as the CLI selector.

The previous `skills` and `targets` fields are deprecated but still accepted for
backward compatibility. `skills: [foo]` is equivalent to `content: [skill/foo]`.

### Removed syntax

- **`--skill` flag** — removed. Use `:skill/name` instead.
- **`#ref` fragment** — removed. Use `@ref` instead.
- **`@skill` shorthand** — removed. Use `:skill/name` instead.

### Parsing

The source string is parsed left to right:

1. Split on `@` — left side is the source, right side (if present) is the git
   ref. Only the first `@` is considered (git refs don't contain `@`).
2. Split the source on `:` — the first segment is the package identifier
   (`owner/repo` or local path), subsequent segments are content selectors.
3. Each content selector is validated as `type/name` where type is one of the
   six recognized content types.

```
owner/repo@v2.0.0:skill/k8s:command/scan
└─ source ─┘└ ref ┘└ selector ┘└ selector ┘
```

### Interaction with the install pipeline

Content selectors filter metadata entries before passing them to drivers
(ADR-009). The pipeline:

1. Read metadata entries from the package
2. If selectors are present, filter to entries matching `type` and `name`
3. Filter remaining entries by driver's `SupportedTypes()`
4. Pass filtered entries to the driver

This means a selector like `:command/scan` on a package installed to Cursor
(which only supports skills) would result in nothing being installed to Cursor —
the command entry passes the selector filter but fails the driver capability
filter. This is correct behavior.

### Delete syntax

```
agentpack del owner/repo                          # remove entire package
agentpack del owner/repo:skill/k8s                # remove one skill
agentpack del owner/repo:command/scan             # remove one command
```

This replaces the previous `@skill` syntax for partial removal.

## Consequences

- CLI syntax aligns with npm/cargo/go conventions for version pinning
- All six content types are selectable, not just skills
- One unified selector concept replaces `--skill` flag, `@skill` shorthand, and
  `#ref` fragment
- Breaking change from the previous `@skill` and `#ref` syntax
- The `skills` field in `agentpack-packages.yaml` is deprecated in favor of
  `content`

## More Information

- Supersedes the `@skill` syntax from ADR-008 content filtering
- Extends [ADR-009](009-metadata-driven-format.md) content type model
- Motivated by
  [vercel-labs/skills#863](https://github.com/vercel-labs/skills/issues/863)
  supply-chain incident caused by ambiguous `@` syntax
