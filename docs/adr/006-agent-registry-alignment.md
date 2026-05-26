# ADR-006: Agent Registry Alignment with agentskills.io

## Status

Accepted

## Decision Drivers

- vercel-labs/skills is the de facto standard for agent skill installation
- Most agents read from `.agents/skills/` (the universal convention)
- Only a few agents have unique install directories
- Our current per-agent targets have incorrect detection and install paths
- One Go package per agent doesn't scale to 50+ agents
- Skills ecosystem inspired the agentskills.io format this project supports

## Considered Alternatives

- **Keep per-agent Go packages** — doesn't scale, each new agent needs a new
  package with boilerplate
- **Copy vercel-labs/skills agent list verbatim** — brittle, their list changes
  frequently
- **Only support universal target** — loses detection for specific agents

## Context

The vercel-labs/skills project (agentskills.io) maintains a registry of 50+ AI
coding agents with their detection paths and skill directories. Most agents read
skills from `.agents/skills/` — the universal convention. Only a few have unique
directories:

| Agent           | Install dir                                                  | Detection               |
| --------------- | ------------------------------------------------------------ | ----------------------- |
| Claude Code     | `.claude/skills/` (+ commands, agents, hooks, mcp, settings) | `~/.claude/`            |
| Windsurf        | `.windsurf/skills/`                                          | `~/.codeium/windsurf`   |
| Everything else | `.agents/skills/`                                            | Agent-specific home dir |

Our current implementation has 6 separate Go packages with hardcoded paths,
several of which are wrong (Copilot detects on `.github/` in cwd, Windsurf
detects on `~/.windsurf/`).

## Decision

### Two tiers of targets

**Tier 1: Dedicated drivers** — agents with unique install paths or special
behavior (config merging, multiple content types):

- `claudecode` — `.claude/{skills,commands,agents}/` + config merging
- `windsurf` — `.windsurf/skills/`

These remain as separate Go packages under `pkg/target/`.

**Tier 2: Data-driven agents** — agents that read from `.agents/skills/` with
agent-specific detection. Defined as data, not code:

```go
var agents = []AgentDef{
    {Name: "cursor", DisplayName: "Cursor", DetectHome: ".cursor"},
    {Name: "copilot", DisplayName: "GitHub Copilot", DetectHome: ".copilot"},
    {Name: "gemini", DisplayName: "Gemini CLI", DetectHome: ".gemini"},
    {Name: "codex", DisplayName: "Codex", DetectHome: ".codex", EnvOverride: "CODEX_HOME"},
    {Name: "opencode", DisplayName: "OpenCode", DetectConfig: "opencode"},
    {Name: "cline", DisplayName: "Cline", DetectHome: ".cline"},
    {Name: "goose", DisplayName: "Goose", DetectConfig: "goose"},
    {Name: "roo", DisplayName: "Roo Code", DetectHome: ".roo"},
    {Name: "amp", DisplayName: "Amp", DetectConfig: "amp"},
    {Name: "continue", DisplayName: "Continue", DetectHome: ".continue"},
    // ... more agents
}
```

Each data-driven agent:

- Detects via `~/{DetectHome}` or `~/.config/{DetectConfig}`
- Installs skills to `.agents/skills/{name}/`
- Shares the universal install implementation

**Tier 3: Universal fallback** — always active, installs to `.agents/skills/`.
Catches any agent not explicitly listed.

### Remove per-agent packages for tier 2

Delete `pkg/target/cursor/`, `pkg/target/copilot/`, `pkg/target/gemini/`.
Replace with a single `pkg/target/agents/` package that registers all
data-driven agents from the list.

### Fix detection paths

Align with vercel-labs/skills:

- Copilot: `~/.copilot` (not `.github/` in cwd)
- Windsurf: `~/.codeium/windsurf` (not `~/.windsurf/`)
- Cursor: `~/.cursor` (correct, keep)
- Gemini: `~/.gemini` (correct, keep)

### Global skills support

Each agent has a global skills directory (e.g. `~/.cursor/skills/`,
`~/.gemini/skills/`). Currently we only do project-local installs. Global
install support is deferred to a future ADR.

## Consequences

- Agent registry scales to 50+ agents via data, not code
- Detection and install paths align with the agentskills.io standard
- Fewer Go packages to maintain (2 dedicated + 1 data-driven)
- New agents added by appending to a list, not writing a package
- vercel-labs/skills can be used as a reference for new agents
- Windsurf detection fix prevents false negatives
- Copilot detection fix prevents false positives

## Influences

- [vercel-labs/skills](https://github.com/vercel-labs/skills) — agent registry,
  detection paths, install conventions
- [agentskills.io](https://agentskills.io) — skill format standard
- [agents.md](https://agents.md) — `.agents/` directory convention
