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

// Package antigravity is the agentpack target driver for Antigravity (Google).
// It installs skills into .agents/skills/ (local) or
// ~/.gemini/antigravity/skills/ (global), merges MCP server configs into
// ~/.gemini/config/mcp_config.json, and merges hooks into hooks.json at the
// project root.
package antigravity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/retr0h/agentpack/internal/driver/fs"
	"github.com/retr0h/agentpack/internal/target"
)

// Antigravity is the target driver for Antigravity (Google).
type Antigravity struct {
	userHomeFunc func() (string, error)
	cwdFunc      func() (string, error)
	mkdirAllFunc func(string, os.FileMode) error
}

// New returns a production Antigravity driver.
func New() *Antigravity {
	return &Antigravity{
		userHomeFunc: os.UserHomeDir,
		cwdFunc:      os.Getwd,
		mkdirAllFunc: os.MkdirAll,
	}
}

// Name returns the target identifier.
func (a *Antigravity) Name() string { return "antigravity" }

// DisplayName returns the human-readable target name.
func (a *Antigravity) DisplayName() string { return "Antigravity" }

// SupportedTypes returns the content types this driver can install.
func (a *Antigravity) SupportedTypes() []string {
	return []string{"skill", "hook", "mcp"}
}

// Detect returns true if the Antigravity home directory exists.
func (a *Antigravity) Detect() bool {
	home, err := a.userHomeFunc()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(home, ".gemini", "antigravity"))

	return err == nil
}

// Install copies content from opts.SourceDir into the correct locations for
// Antigravity. When opts.Entries is non-empty the driver installs only the
// listed entries; otherwise it falls back to the legacy directory-walking
// behaviour. Returns the list of files written.
func (a *Antigravity) Install(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(opts.Entries) > 0 {
		return a.installFromEntries(ctx, opts)
	}

	return a.installFromDirs(ctx, opts)
}

// installFromEntries installs only the content items listed in opts.Entries.
func (a *Antigravity) installFromEntries(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	var allFiles []target.InstalledFile

	for _, entry := range opts.Entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		switch entry.Type {
		case "skill":
			written, err := a.installSkillEntry(ctx, opts, entry)
			if err != nil {
				return nil, err
			}

			allFiles = append(allFiles, written...)

		case "mcp":
			mcpPath, err := a.mcpConfigPath()
			if err != nil {
				return nil, err
			}

			if err := fs.InstallMCP(ctx, opts.SourceDir, mcpPath); err != nil {
				return nil, err
			}

		case "hook":
			hooksPath, err := a.hooksPath(opts)
			if err != nil {
				return nil, err
			}

			if err := fs.InstallHooksJSON(ctx, opts.SourceDir, hooksPath, opts.Name); err != nil {
				return nil, err
			}
		}
	}

	return allFiles, nil
}

// installFromDirs walks convention-named directories under opts.SourceDir
// and installs everything found. This is the legacy fallback when no manifest
// entries are provided. Skills, MCP, and hooks are handled.
func (a *Antigravity) installFromDirs(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	baseDir, skillsDir, err := fs.ResolveDirs(
		opts,
		".gemini/antigravity/skills",
		".agents/skills",
		a.userHomeFunc,
		a.cwdFunc,
	)
	if err != nil {
		return nil, err
	}

	destDir := filepath.Join(skillsDir, opts.Name)

	if err := a.mkdirAllFunc(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir skills dir: %w", err)
	}

	skillsSrc := filepath.Join(opts.SourceDir, "skills")
	if err := fs.CopyTreeIfExists(ctx, skillsSrc, destDir); err != nil {
		return nil, fmt.Errorf("copy skills: %w", err)
	}

	files, err := fs.EnumerateFiles(ctx, destDir, baseDir)
	if err != nil {
		return nil, fmt.Errorf("enumerate installed files: %w", err)
	}

	mcpPath, mcpErr := a.mcpConfigPath()
	if mcpErr != nil {
		return nil, mcpErr
	}

	if err := fs.InstallMCP(ctx, opts.SourceDir, mcpPath); err != nil {
		return nil, err
	}

	hooksPath, hooksErr := a.hooksPath(opts)
	if hooksErr != nil {
		return nil, hooksErr
	}

	if err := fs.InstallHooksJSON(ctx, opts.SourceDir, hooksPath, opts.Name); err != nil {
		return nil, err
	}

	return files, nil
}

// installSkillEntry copies a single skill entry's tree into the skills
// directory.
func (a *Antigravity) installSkillEntry(
	ctx context.Context,
	opts target.InstallOpts,
	entry target.ContentEntry,
) ([]target.InstalledFile, error) {
	baseDir, skillsDir, err := fs.ResolveDirs(
		opts,
		".gemini/antigravity/skills",
		".agents/skills",
		a.userHomeFunc,
		a.cwdFunc,
	)
	if err != nil {
		return nil, err
	}

	return fs.InstallSkillEntry(ctx, entry, skillsDir, baseDir, a.mkdirAllFunc)
}

// mcpConfigPath returns the global MCP config path at
// ~/.gemini/config/mcp_config.json.
func (a *Antigravity) mcpConfigPath() (string, error) {
	home, err := a.userHomeFunc()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}

	return filepath.Join(home, ".gemini", "config", "mcp_config.json"), nil
}

// hooksPath returns the hooks.json path for the install scope. Project
// installs use hooks.json at the project root.
func (a *Antigravity) hooksPath(opts target.InstallOpts) (string, error) {
	dir := opts.Dir
	if dir == "" {
		cwd, err := a.cwdFunc()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return filepath.Join(dir, "hooks.json"), nil
}

// List returns nil; Antigravity does not store managed-plugin metadata.
func (a *Antigravity) List() ([]target.InstalledPlugin, error) {
	return nil, nil
}
