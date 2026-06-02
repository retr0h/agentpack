# Agent Feature Matrix

Driver support for content types beyond skills. Only features verified against
documentation or source code are listed. Unverified claims are not included.

**Legend:** `driver` = dedicated driver implemented, `planned` = verified but
not yet implemented, `-` = not supported or not verified

## Tier 1 — Dedicated Drivers

| Agent       | Skills | Commands | Hooks  | Agents | MCP    | Config | Driver                         |
| ----------- | ------ | -------- | ------ | ------ | ------ | ------ | ------------------------------ |
| Claude Code | driver | driver   | driver | driver | driver | driver | `internal/driver/claudecode/`  |
| Cursor      | driver | -        | -      | -      | driver | -      | `internal/driver/cursor/`      |
| Windsurf    | driver | -        | driver | -      | driver | -      | `internal/driver/windsurf/`    |
| Codex       | driver | -        | driver | -      | -      | -      | `internal/driver/codex/`       |
| Copilot     | driver | -        | -      | -      | driver | -      | `internal/driver/copilot/`     |
| Cline       | driver | -        | driver | -      | driver | -      | `internal/driver/cline/`       |
| Gemini CLI  | driver | -        | -      | -      | driver | -      | `internal/driver/gemini/`      |
| Devin       | driver | -        | driver | -      | driver | -      | `internal/driver/devin/`       |
| Goose       | driver | -        | -      | -      | -      | -      | `internal/driver/goose/`       |
| Continue    | driver | -        | -      | -      | driver | -      | `internal/driver/continuedev/` |
| Kiro        | driver | -        | -      | -      | -      | -      | `internal/driver/kiro/`        |
| Amp         | driver | -        | -      | -      | driver | -      | `internal/driver/amp/`         |
| Roo         | driver | -        | -      | -      | -      | -      | `internal/driver/roo/`         |

## Tier 2 — Generic Driver (Skills Only)

All remaining agents use the generic data-driven driver at
`internal/driver/agents/`. They support `skill` only via
`SupportedTypes() = ["skill"]`. When an agent gains file-based support for
additional types, it moves to Tier 1 and gets a dedicated driver.

## Verified File Paths

Reference for driver implementation. Each path is verified against official
documentation.

### Cursor

- **MCP**: `.cursor/mcp.json` (project), `~/.cursor/mcp.json` (global)
- **Format**:
  `{"mcpServers": {"name": {"command": "...", "args": [...], "env": {...}}}}` —
  same as Claude Code

### Windsurf

- **MCP**: `~/.codeium/windsurf/mcp_config.json` (global only)
- **Hooks**: `.windsurf/hooks.json` (project), `~/.codeium/windsurf/hooks.json`
  (global)
- **Format**: Same `mcpServers` structure as Claude Code. Hooks have `command`,
  `showOutput`, `workingDirectory` fields.

### Codex

- **Hooks**: `hooks.json` alongside config layers, or `[[hooks.PreToolUse]]`
  inline in `.codex/config.toml`
- **Config**: `.codex/config.toml` (project), walks from root to cwd
- **Events**: PreToolUse, PostToolUse, PermissionRequest, PreCompact,
  PostCompact, UserPromptSubmit, Stop, SessionStart, SubagentStart, SubagentStop

### Copilot

- **MCP (CLI)**: `~/.copilot/mcp-config.json` (global),
  `.copilot/mcp-config.json` (project)
- **MCP (VS Code)**: `.vscode/mcp.json` — uses `"servers"` key instead of
  `"mcpServers"`
- **Format (CLI)**:
  `{"mcpServers": {"name": {"type": "local", "command": "...", "args": [...]}}}`
  — adds `type` and `tools` fields

### Cline

- **MCP**: `~/.cline/data/settings/cline_mcp_settings.json` (CLI), VS Code
  globalStorage path (extension)
- **Hooks**: `.clinerules/hooks/` (project), `~/Documents/Cline/Rules/Hooks/`
  (global) — executable scripts, not JSON
- **Events**: preToolUse, postToolUse, userPromptSubmit, taskStart
- **Format**: MCP uses same `mcpServers` structure. Hooks are executable scripts
  receiving JSON on stdin.

### Gemini CLI

- **MCP**: `settings.json` with MCP server config
- **Agents**: `.gemini/agents/` (project), `~/.gemini/agents/` (global) —
  Markdown with YAML frontmatter

### Devin

- **MCP**: `.devin/config.json` with MCP section
- **Hooks**: `.devin/hooks.v1.json` — compatible with Claude Code hooks format
- **Agents**: `.devin/agents/` for subagent profiles
- **Config**: `.devin/config.json` (project), `.devin/config.local.json`
  (personal)

### Goose

- **MCP**: `~/.config/goose/config.yaml` with extensions/MCP
- **Hooks**: Supported since v1.34.0
- **Config**: `~/.config/goose/config.yaml`

### Continue

- **MCP**: `.continue/mcpServers/` directory with YAML/JSON config files per
  server
- **Config**: `~/.continue/config.yaml` (global)

### Kiro

- **MCP**: Configured in agent JSON configs
- **Hooks**: agentSpawn, userPromptSubmit, preToolUse, postToolUse — with
  matchers
- **Agents**: `.kiro/agents/` (project), `~/.kiro/agents/` (global) — JSON
  configs

### Amp

- **MCP**: `mcp.json` bundled with skills, `.amp/settings.json` (project),
  `~/.config/amp/settings.json` (global)
- **Config**: `settings.json` at project or global level

### Roo

- **Config**: `.roomodes` YAML for custom modes, `.roo/rules/` for mode-specific
  rules
