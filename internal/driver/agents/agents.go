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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/retr0h/agentpack/internal/driver/fs"
	"github.com/retr0h/agentpack/pkg/target"
)

// AgentDef describes a data-driven agent target.
type AgentDef struct {
	Name            string
	Display         string
	DetectHome      string // check ~/X exists
	DetectConfig    string // check ~/.config/X exists
	EnvOverride     string // env var for home override
	AlwaysDetect    bool   // true for the universal fallback
	GlobalSkillsDir string // relative to home, e.g. ".cursor/skills"
	LocalSkillsDir  string // relative to cwd, empty = ".agents/skills" (default)
}

var registry = []AgentDef{
	{Name: "adal", Display: "AdaL", DetectHome: ".adal", GlobalSkillsDir: ".adal/skills"},
	{
		Name:            "aider-desk",
		Display:         "AiderDesk",
		DetectHome:      ".aider-desk",
		GlobalSkillsDir: ".aider-desk/skills",
	},
	{
		Name:            "augment",
		Display:         "Augment",
		DetectHome:      ".augment",
		GlobalSkillsDir: ".augment/skills",
	},
	{Name: "bob", Display: "IBM Bob", DetectHome: ".bob", GlobalSkillsDir: ".bob/skills"},
	{
		Name:            "codearts-agent",
		Display:         "CodeArts Agent",
		DetectHome:      ".codeartsdoer",
		GlobalSkillsDir: ".codeartsdoer/skills",
	},
	{
		Name:            "codemaker",
		Display:         "Codemaker",
		DetectHome:      ".codemaker",
		GlobalSkillsDir: ".codemaker/skills",
	},
	{
		Name:            "codestudio",
		Display:         "Code Studio",
		DetectHome:      ".codestudio",
		GlobalSkillsDir: ".codestudio/skills",
	},
	{
		Name:            "command-code",
		Display:         "Command Code",
		DetectHome:      ".commandcode",
		GlobalSkillsDir: ".commandcode/skills",
	},
	{
		Name:            "deepagents",
		Display:         "Deep Agents",
		DetectHome:      ".deepagents",
		GlobalSkillsDir: ".deepagents/agent/skills",
	},
	{Name: "dexto", Display: "Dexto", DetectHome: ".dexto", GlobalSkillsDir: ".dexto/skills"},
	{
		Name:            "iflow-cli",
		Display:         "iFlow CLI",
		DetectHome:      ".iflow",
		GlobalSkillsDir: ".iflow/skills",
	},
	{
		Name:            "kilo",
		Display:         "Kilo Code",
		DetectHome:      ".kilocode",
		GlobalSkillsDir: ".kilocode/skills",
	},
	{Name: "kode", Display: "Kode", DetectHome: ".kode", GlobalSkillsDir: ".kode/skills"},
	{Name: "mcpjam", Display: "MCPJam", DetectHome: ".mcpjam", GlobalSkillsDir: ".mcpjam/skills"},
	{
		Name:            "mistral-vibe",
		Display:         "Mistral Vibe",
		DetectHome:      ".vibe",
		EnvOverride:     "VIBE_HOME",
		GlobalSkillsDir: ".vibe/skills",
	},
	{Name: "mux", Display: "Mux", DetectHome: ".mux", GlobalSkillsDir: ".mux/skills"},
	{
		Name:            "neovate",
		Display:         "Neovate",
		DetectHome:      ".neovate",
		GlobalSkillsDir: ".neovate/skills",
	},
	{
		Name:            "openclaw",
		Display:         "OpenClaw",
		DetectHome:      ".openclaw",
		GlobalSkillsDir: ".openclaw/skills",
	},
	{Name: "pi", Display: "Pi", DetectHome: ".pi/agent", GlobalSkillsDir: ".pi/agent/skills"},
	{Name: "pochi", Display: "Pochi", DetectHome: ".pochi", GlobalSkillsDir: ".pochi/skills"},
	{Name: "qoder", Display: "Qoder", DetectHome: ".qoder", GlobalSkillsDir: ".qoder/skills"},
	{Name: "qwen-code", Display: "Qwen Code", DetectHome: ".qwen", GlobalSkillsDir: ".qwen/skills"},
	{
		Name:            "replit",
		Display:         "Replit",
		DetectHome:      ".replit",
		GlobalSkillsDir: ".config/agents/skills",
	},
	{
		Name:            "tabnine-cli",
		Display:         "Tabnine CLI",
		DetectHome:      ".tabnine",
		GlobalSkillsDir: ".tabnine/agent/skills",
	},
	{Name: "trae", Display: "Trae", DetectHome: ".trae", GlobalSkillsDir: ".trae/skills"},
	{
		Name:            "trae-cn",
		Display:         "Trae CN",
		DetectHome:      ".trae-cn",
		GlobalSkillsDir: ".trae-cn/skills",
	},
	{
		Name:            "zencoder",
		Display:         "Zencoder",
		DetectHome:      ".zencoder",
		GlobalSkillsDir: ".zencoder/skills",
	},
	{
		Name:            "universal",
		Display:         "Universal",
		AlwaysDetect:    true,
		GlobalSkillsDir: ".config/agents/skills",
	},
}

// RegisterAll registers every data-driven agent in the global target registry.
func RegisterAll() {
	for _, def := range registry {
		target.Register(newAgent(def, os.UserHomeDir, os.Getwd))
	}
}

// Defs returns all registered agent definitions.
func Defs() []AgentDef {
	return registry
}

// agent is a data-driven target that installs to .agents/skills/{name}/.
type agent struct {
	def               AgentDef
	userHomeFunc      func() (string, error)
	userConfigDirFunc func() (string, error)
	cwdFunc           func() (string, error)
	getenvFunc        func(string) string
}

func newAgent(
	def AgentDef,
	homeFunc func() (string, error),
	cwdFunc func() (string, error),
) *agent {
	return &agent{
		def:               def,
		userHomeFunc:      homeFunc,
		userConfigDirFunc: os.UserConfigDir,
		cwdFunc:           cwdFunc,
		getenvFunc:        os.Getenv,
	}
}

// Name returns the agent identifier.
func (a *agent) Name() string { return a.def.Name }

// DisplayName returns the human-readable agent name.
func (a *agent) DisplayName() string { return a.def.Display }

// SupportedTypes returns the content types this driver can install.
func (a *agent) SupportedTypes() []string {
	return []string{"skill"}
}

// Detect returns true when the agent's home or config directory exists.
func (a *agent) Detect() bool {
	if a.def.AlwaysDetect {
		return true
	}

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
		configDir, err := a.userConfigDirFunc()
		if err != nil {
			return false
		}
		_, err = os.Stat(filepath.Join(configDir, a.def.DetectConfig))
		return err == nil
	}

	return false
}

// Install copies skills into the appropriate skills directory for this agent.
// When opts.Global is true it installs into the agent's global skills directory
// under the user's home. Otherwise it installs into the project-local directory.
// When opts.Entries is non-empty, only those entries are installed; otherwise
// the legacy directory-walking behaviour is used.
// Returns the list of files written.
func (a *agent) Install(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(opts.Entries) > 0 {
		return a.installFromEntries(ctx, opts)
	}

	var destDir string
	var baseDir string

	if opts.Global {
		home, err := a.userHomeFunc()
		if err != nil {
			return nil, fmt.Errorf("home dir: %w", err)
		}

		baseDir = home
		destDir = filepath.Join(home, a.def.GlobalSkillsDir, opts.Name)
	} else {
		cwd, err := a.cwdFunc()
		if err != nil {
			return nil, fmt.Errorf("getwd: %w", err)
		}

		baseDir = cwd
		localDir := ".agents/skills"
		if a.def.LocalSkillsDir != "" {
			localDir = a.def.LocalSkillsDir
		}

		destDir = filepath.Join(cwd, localDir, opts.Name)
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir agents skills dir: %w", err)
	}

	skillsSrc := filepath.Join(opts.SourceDir, "skills")

	if err := fs.CopyTreeIfExists(ctx, skillsSrc, destDir); err != nil {
		return nil, fmt.Errorf("copy skills: %w", err)
	}

	var files []target.InstalledFile

	if err := filepath.WalkDir(destDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		rel, relErr := filepath.Rel(baseDir, path)
		if relErr != nil {
			return relErr
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		h := sha256.Sum256(data)
		files = append(files, target.InstalledFile{
			Path:   rel,
			SHA256: hex.EncodeToString(h[:]),
		})

		return nil
	}); err != nil {
		return nil, fmt.Errorf("enumerate installed files: %w", err)
	}

	return files, nil
}

// installFromEntries installs only the content items listed in opts.Entries.
// Each entry's Root tree is copied into {skillsDir}/{entry.Name}/.
func (a *agent) installFromEntries(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	var baseDir string
	var skillsDir string

	if opts.Global {
		home, err := a.userHomeFunc()
		if err != nil {
			return nil, fmt.Errorf("home dir: %w", err)
		}

		baseDir = home
		skillsDir = filepath.Join(home, a.def.GlobalSkillsDir)
	} else {
		cwd, err := a.cwdFunc()
		if err != nil {
			return nil, fmt.Errorf("getwd: %w", err)
		}

		baseDir = cwd
		localDir := ".agents/skills"
		if a.def.LocalSkillsDir != "" {
			localDir = a.def.LocalSkillsDir
		}

		skillsDir = filepath.Join(cwd, localDir)
	}

	var allFiles []target.InstalledFile

	for _, entry := range opts.Entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		srcDir := entry.Root
		destDir := filepath.Join(skillsDir, entry.Name)

		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir agents skills dir: %w", err)
		}

		if err := fs.CopyTreeIfExists(ctx, srcDir, destDir); err != nil {
			return nil, fmt.Errorf("copy skills: %w", err)
		}

		if err := filepath.WalkDir(destDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}

			rel, relErr := filepath.Rel(baseDir, path)
			if relErr != nil {
				return relErr
			}

			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}

			h := sha256.Sum256(data)
			allFiles = append(allFiles, target.InstalledFile{
				Path:   rel,
				SHA256: hex.EncodeToString(h[:]),
			})

			return nil
		}); err != nil {
			return nil, fmt.Errorf("enumerate installed files: %w", err)
		}
	}

	return allFiles, nil
}

// List returns nil; data-driven agents do not store managed-plugin metadata.
func (a *agent) List() ([]target.InstalledPlugin, error) {
	return nil, nil
}
