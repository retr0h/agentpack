// Package driver registers all built-in target drivers.
// Import this package with a blank import to register every driver:
//
//	_ "github.com/retr0h/agentpack/internal/driver"
package driver

import (
	_ "github.com/retr0h/agentpack/internal/driver/agents"
	_ "github.com/retr0h/agentpack/internal/driver/amp"
	_ "github.com/retr0h/agentpack/internal/driver/claudecode"
	_ "github.com/retr0h/agentpack/internal/driver/cline"
	_ "github.com/retr0h/agentpack/internal/driver/codex"
	_ "github.com/retr0h/agentpack/internal/driver/continuedev"
	_ "github.com/retr0h/agentpack/internal/driver/copilot"
	_ "github.com/retr0h/agentpack/internal/driver/cursor"
	_ "github.com/retr0h/agentpack/internal/driver/devin"
	_ "github.com/retr0h/agentpack/internal/driver/gemini"
	_ "github.com/retr0h/agentpack/internal/driver/goose"
	_ "github.com/retr0h/agentpack/internal/driver/kiro"
	_ "github.com/retr0h/agentpack/internal/driver/roo"
	_ "github.com/retr0h/agentpack/internal/driver/windsurf"
)
