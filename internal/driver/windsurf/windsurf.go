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

// Package windsurf is the agentpack target driver for Windsurf.
// It installs skills into .windsurf/skills/ (local) or
// ~/.codeium/windsurf/skills/ (global), merges MCP server configs into
// ~/.codeium/windsurf/mcp_config.json (global only), and merges hooks into
// .windsurf/hooks.json (local) or ~/.codeium/windsurf/hooks.json (global).
package windsurf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/retr0h/agentpack/internal/driver"
	"github.com/retr0h/agentpack/internal/target"
)

// Windsurf is the target driver for Windsurf.
type Windsurf struct {
	userHomeFunc func() (string, error)
	cwdFunc      func() (string, error)
	mkdirAllFunc func(string, os.FileMode) error
}

// New returns a production Windsurf driver.
func New() *Windsurf {
	return &Windsurf{
		userHomeFunc: os.UserHomeDir,
		cwdFunc:      os.Getwd,
		mkdirAllFunc: os.MkdirAll,
	}
}

// Name returns the target identifier.
func (w *Windsurf) Name() string { return "windsurf" }

// DisplayName returns the human-readable target name.
func (w *Windsurf) DisplayName() string { return "Windsurf" }

// SupportedTypes returns the content types this driver can install.
func (w *Windsurf) SupportedTypes() []string {
	return []string{"skill", "mcp", "hook"}
}

// Detect returns true if the Windsurf config directory exists.
func (w *Windsurf) Detect() bool {
	home, err := w.userHomeFunc()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(home, ".codeium", "windsurf"))

	return err == nil
}

// Install copies content from opts.SourceDir into the correct locations for
// Windsurf. When opts.Entries is non-empty the driver installs only the listed
// entries; otherwise it falls back to the legacy directory-walking behaviour
// (skills only). Returns the list of files written.
func (w *Windsurf) Install(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(opts.Entries) > 0 {
		return w.installFromEntries(ctx, opts)
	}

	return w.installFromDirs(ctx, opts)
}

// installFromEntries installs only the content items listed in opts.Entries.
func (w *Windsurf) installFromEntries(
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
			written, err := w.installSkillEntry(ctx, opts, entry)
			if err != nil {
				return nil, err
			}

			allFiles = append(allFiles, written...)

		case "mcp":
			mcpPath, err := w.mcpConfigPath()
			if err != nil {
				return nil, err
			}

			if err := driver.InstallMCP(ctx, opts.SourceDir, mcpPath); err != nil {
				return nil, err
			}

		case "hook":
			hooksPath, err := w.hooksPath(opts)
			if err != nil {
				return nil, err
			}

			if err := driver.InstallHooksJSON(ctx, opts.SourceDir, hooksPath, opts.Name); err != nil {
				return nil, err
			}
		}
	}

	return allFiles, nil
}

// installFromDirs walks convention-named directories under opts.SourceDir
// and installs everything found. This is the legacy fallback when no manifest
// entries are provided. Skills, MCP, and hooks are handled.
func (w *Windsurf) installFromDirs(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	baseDir, skillsDir, err := driver.ResolveDirs(
		opts,
		".codeium/windsurf/skills",
		".windsurf/skills",
		w.userHomeFunc,
		w.cwdFunc,
	)
	if err != nil {
		return nil, err
	}

	destDir := filepath.Join(skillsDir, opts.Name)

	if err := w.mkdirAllFunc(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir skills dir: %w", err)
	}

	skillsSrc := filepath.Join(opts.SourceDir, "skills")
	if err := driver.CopyTreeIfExists(ctx, skillsSrc, destDir); err != nil {
		return nil, fmt.Errorf("copy skills: %w", err)
	}

	files, err := driver.EnumerateFiles(ctx, destDir, baseDir)
	if err != nil {
		return nil, fmt.Errorf("enumerate installed files: %w", err)
	}

	mcpPath, mcpErr := w.mcpConfigPath()
	if mcpErr != nil {
		return nil, mcpErr
	}

	if err := driver.InstallMCP(ctx, opts.SourceDir, mcpPath); err != nil {
		return nil, err
	}

	hooksPath, hooksErr := w.hooksPath(opts)
	if hooksErr != nil {
		return nil, hooksErr
	}

	if err := driver.InstallHooksJSON(ctx, opts.SourceDir, hooksPath, opts.Name); err != nil {
		return nil, err
	}

	return files, nil
}

// installSkillEntry copies a single skill entry's tree into the skills
// directory.
func (w *Windsurf) installSkillEntry(
	ctx context.Context,
	opts target.InstallOpts,
	entry target.ContentEntry,
) ([]target.InstalledFile, error) {
	baseDir, skillsDir, err := driver.ResolveDirs(
		opts,
		".codeium/windsurf/skills",
		".windsurf/skills",
		w.userHomeFunc,
		w.cwdFunc,
	)
	if err != nil {
		return nil, err
	}

	return driver.InstallSkillEntry(ctx, entry, skillsDir, baseDir, w.mkdirAllFunc)
}

// mcpConfigPath returns the global MCP config path. Windsurf MCP is global
// only — there is no project-level MCP configuration.
func (w *Windsurf) mcpConfigPath() (string, error) {
	home, err := w.userHomeFunc()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}

	return filepath.Join(home, ".codeium", "windsurf", "mcp_config.json"), nil
}

// hooksPath returns the hooks.json path for the install scope. Project
// installs use .windsurf/hooks.json; global installs use
// ~/.codeium/windsurf/hooks.json.
func (w *Windsurf) hooksPath(opts target.InstallOpts) (string, error) {
	if opts.Global {
		home, err := w.userHomeFunc()
		if err != nil {
			return "", fmt.Errorf("home dir: %w", err)
		}

		return filepath.Join(home, ".codeium", "windsurf", "hooks.json"), nil
	}

	dir := opts.Dir
	if dir == "" {
		cwd, err := w.cwdFunc()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return filepath.Join(dir, ".windsurf", "hooks.json"), nil
}

// List returns nil; Windsurf does not store managed-plugin metadata.
func (w *Windsurf) List() ([]target.InstalledPlugin, error) {
	return nil, nil
}
