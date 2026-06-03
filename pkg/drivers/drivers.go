// Package drivers wires up and registers every built-in target driver. It is
// the composition root: import it to make all built-in drivers available via
// the target registry.
package drivers

import (
	"sync"

	"github.com/retr0h/agentpack/internal/driver/agents"
	"github.com/retr0h/agentpack/internal/driver/amp"
	"github.com/retr0h/agentpack/internal/driver/antigravity"
	"github.com/retr0h/agentpack/internal/driver/claudecode"
	"github.com/retr0h/agentpack/internal/driver/cline"
	"github.com/retr0h/agentpack/internal/driver/codebuddy"
	"github.com/retr0h/agentpack/internal/driver/codex"
	"github.com/retr0h/agentpack/internal/driver/continuedev"
	"github.com/retr0h/agentpack/internal/driver/copilot"
	"github.com/retr0h/agentpack/internal/driver/cortex"
	"github.com/retr0h/agentpack/internal/driver/crush"
	"github.com/retr0h/agentpack/internal/driver/cursor"
	"github.com/retr0h/agentpack/internal/driver/devin"
	"github.com/retr0h/agentpack/internal/driver/droid"
	"github.com/retr0h/agentpack/internal/driver/firebender"
	"github.com/retr0h/agentpack/internal/driver/forgecode"
	"github.com/retr0h/agentpack/internal/driver/gemini"
	"github.com/retr0h/agentpack/internal/driver/goose"
	"github.com/retr0h/agentpack/internal/driver/hermes"
	"github.com/retr0h/agentpack/internal/driver/junie"
	"github.com/retr0h/agentpack/internal/driver/kimi"
	"github.com/retr0h/agentpack/internal/driver/kiro"
	"github.com/retr0h/agentpack/internal/driver/opencode"
	"github.com/retr0h/agentpack/internal/driver/openhands"
	"github.com/retr0h/agentpack/internal/driver/roo"
	"github.com/retr0h/agentpack/internal/driver/rovodev"
	"github.com/retr0h/agentpack/internal/driver/warp"
	"github.com/retr0h/agentpack/internal/driver/windsurf"
	"github.com/retr0h/agentpack/internal/target"
)

var once sync.Once //nolint:gochecknoglobals

// RegisterAll registers every built-in target driver with the global target
// registry. It is safe to call multiple times; only the first call performs
// registration.
func RegisterAll() {
	once.Do(func() {
		target.Register(claudecode.New())
		target.Register(cursor.New())
		target.Register(copilot.New())
		target.Register(windsurf.New())
		target.Register(codex.New())
		target.Register(cline.New())
		target.Register(gemini.New())
		target.Register(devin.New())
		target.Register(forgecode.New())
		target.Register(continuedev.New())
		target.Register(amp.New())
		target.Register(antigravity.New())
		target.Register(goose.New())
		target.Register(hermes.New())
		target.Register(kiro.New())
		target.Register(openhands.New())
		target.Register(junie.New())
		target.Register(kimi.New())
		target.Register(cortex.New())
		target.Register(crush.New())
		target.Register(droid.New())
		target.Register(firebender.New())
		target.Register(rovodev.New())
		target.Register(codebuddy.New())
		target.Register(opencode.New())
		target.Register(warp.New())
		target.Register(roo.New())
		agents.RegisterAll()
	})
}
