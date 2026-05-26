// Copyright (c) 2026 John Dewey

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

// Package agents registers data-driven target drivers for AI coding agents
// that install skills into .agents/skills/{name}/ (the universal convention).
// Each agent is detected via its home or config directory.
package agents

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/retr0h/agentpack/pkg/target"
)

// AgentDef describes a data-driven agent target.
type AgentDef struct {
	Name         string
	Display      string
	DetectHome   string // check ~/X exists
	DetectConfig string // check ~/.config/X exists
	EnvOverride  string // env var for home override
}

var registry = []AgentDef{
	{Name: "cursor", Display: "Cursor", DetectHome: ".cursor"},
	{Name: "copilot", Display: "GitHub Copilot", DetectHome: ".copilot"},
	{Name: "gemini", Display: "Gemini CLI", DetectHome: ".gemini"},
	{Name: "codex", Display: "Codex", DetectHome: ".codex", EnvOverride: "CODEX_HOME"},
	{Name: "opencode", Display: "OpenCode", DetectConfig: "opencode"},
	{Name: "cline", Display: "Cline", DetectHome: ".cline"},
	{Name: "goose", Display: "Goose", DetectConfig: "goose"},
	{Name: "roo", Display: "Roo Code", DetectHome: ".roo"},
	{Name: "amp", Display: "Amp", DetectConfig: "amp"},
	{Name: "continue", Display: "Continue", DetectHome: ".continue"},
	{Name: "kiro", Display: "Kiro", DetectHome: ".kiro"},
	{Name: "devin", Display: "Devin", DetectConfig: "devin"},
	{Name: "warp", Display: "Warp", DetectHome: ".warp"},
	{Name: "trae", Display: "Trae", DetectHome: ".trae"},
}

func init() {
	for _, def := range registry {
		target.Register(newAgent(def, os.UserHomeDir, os.Getwd))
	}
}

// agent is a data-driven target that installs to .agents/skills/{name}/.
type agent struct {
	def          AgentDef
	userHomeFunc func() (string, error)
	cwdFunc      func() (string, error)
	getenvFunc   func(string) string
}

func newAgent(def AgentDef, homeFunc func() (string, error), cwdFunc func() (string, error)) *agent {
	return &agent{def: def, userHomeFunc: homeFunc, cwdFunc: cwdFunc, getenvFunc: os.Getenv}
}

// Name returns the agent identifier.
func (a *agent) Name() string { return a.def.Name }

// DisplayName returns the human-readable agent name.
func (a *agent) DisplayName() string { return a.def.Display }

// Detect returns true when the agent's home or config directory exists.
func (a *agent) Detect() bool {
	home, err := a.userHomeFunc()
	if err != nil {
		return false
	}

	if a.def.EnvOverride != "" {
		if override := a.getenvFunc(a.def.EnvOverride); override != "" {
			_, err := os.Stat(override)
			return err == nil
		}
	}

	if a.def.DetectHome != "" {
		_, err := os.Stat(filepath.Join(home, a.def.DetectHome))
		return err == nil
	}

	if a.def.DetectConfig != "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return false
		}
		_, err = os.Stat(filepath.Join(configDir, a.def.DetectConfig))
		return err == nil
	}

	return false
}

// Install copies skills into .agents/skills/{opts.Name}/ under the project
// directory.
func (a *agent) Install(ctx context.Context, opts target.InstallOpts) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	cwd, err := a.cwdFunc()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}

	destDir := filepath.Join(cwd, ".agents", "skills", opts.Name)

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("mkdir agents skills dir: %w", err)
	}

	skillsSrc := filepath.Join(opts.SourceDir, "skills")

	if err := copyTreeIfExists(ctx, skillsSrc, destDir); err != nil {
		return fmt.Errorf("copy skills: %w", err)
	}

	return nil
}

// List returns nil; data-driven agents do not store managed-plugin metadata.
func (a *agent) List() ([]target.InstalledPlugin, error) {
	return nil, nil
}

func copyTreeIfExists(ctx context.Context, src string, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}

	return filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("rel path: %w", err)
		}

		tgt := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(tgt, 0o755)
		}

		return copyFile(path, tgt)
	})
}

func copyFile(src string, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}

	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}

	if err := os.WriteFile(dst, data, info.Mode()); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}

	return nil
}
