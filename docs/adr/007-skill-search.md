# ADR-007: Skill Search and Discovery

## Status

Accepted

## Decision Drivers

- Users need to discover available skills before adding them
- vercel-labs/skills provides `find`/`search` against skills.sh registry
- APK has `apk search` — our CLI vocabulary should include search
- Discoverability is critical for ecosystem adoption

## Considered Alternatives

- **No search (add by URL only)** — requires users to know the exact repo URL,
  no discoverability, limits adoption
- **Local search only** — search installed skills, not useful for finding new
  ones
- **Embed a full registry** — ship a skills database with the binary, goes
  stale, increases binary size
- **Search skills.sh API** — query the agentskills.io registry at runtime,
  always fresh, no local state

## Context

Currently users must know the exact git URL to add a skill:
`agentpack add github.com/org/repo`. There is no way to browse or search for
available skills. The agentskills.io ecosystem maintains a registry at skills.sh
that can be queried.

## Decision

### New `search` command

```
agentpack search [query]          # search by keyword
agentpack search                  # browse all (interactive)
agentpack search typescript       # filter by keyword
agentpack search --json           # machine-readable output
```

### Registry source

Query the agentskills.io registry API. The registry URL is configurable but
defaults to `https://skills.sh/api/search`.

### Output format

```
  typescript-skills    vercel-labs/agent-skills    TypeScript development patterns
  react-testing        org/react-skills            React testing utilities
  security-review      anthropic/security-skills   Security code review

3 skills found
```

Each result shows: skill name, source repo, description. Accent color for names,
muted for repos and descriptions.

### Integration with `add`

Search results can be piped or the user can copy the source:

```
agentpack search react
  # shows results
agentpack add vercel-labs/agent-skills --skill react-testing
```

### Offline behavior

Search requires network access. When offline, return a clear error:
`"search requires network access"`.

### `-o json` support

```json
[
  {
    "name": "typescript-skills",
    "source": "vercel-labs/agent-skills",
    "description": "TypeScript development patterns"
  }
]
```

## Consequences

- Users can discover skills without leaving the terminal
- Ecosystem adoption improves with discoverability
- Network dependency for search (all other commands work offline)
- Registry API becomes a dependency — needs fallback for downtime
- Aligns with APK (`apk search`) and skills (`npx skills find`) conventions
