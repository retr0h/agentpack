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

// Package openhands is the agentpack target driver for OpenHands.
// It installs skills into .agents/skills/ (local) or ~/.openhands/skills/
// (global), merges hooks into hooks.json (local) or ~/.openhands/hooks.json
// (global), and merges MCP server configs into .mcp.json (local) or
// ~/.openhands/.mcp.json (global).
package openhands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/retr0h/agentpack/internal/driver/fs"
	"github.com/retr0h/agentpack/pkg/target"
)

// OpenHands is the target driver for OpenHands.
type OpenHands struct {
	userHomeFunc func() (string, error)
	cwdFunc      func() (string, error)
	mkdirAllFunc func(string, os.FileMode) error
}

// New returns a production OpenHands driver.
func New() *OpenHands {
	return &OpenHands{
		userHomeFunc: os.UserHomeDir,
		cwdFunc:      os.Getwd,
		mkdirAllFunc: os.MkdirAll,
	}
}

// Name returns the target identifier.
func (o *OpenHands) Name() string { return "openhands" }

// DisplayName returns the human-readable target name.
func (o *OpenHands) DisplayName() string { return "OpenHands" }

// SupportedTypes returns the content types this driver can install.
func (o *OpenHands) SupportedTypes() []string {
	return []string{"skill", "hook", "mcp"}
}

// Detect returns true if the OpenHands home directory exists.
func (o *OpenHands) Detect() bool {
	home, err := o.userHomeFunc()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(home, ".openhands"))

	return err == nil
}

// Install copies content from opts.SourceDir into the correct locations for
// OpenHands. When opts.Entries is non-empty the driver installs only the listed
// entries; otherwise it falls back to the legacy directory-walking behaviour
// (skills, hooks, and MCP). Returns the list of files written.
func (o *OpenHands) Install(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(opts.Entries) > 0 {
		return o.installFromEntries(ctx, opts)
	}

	return o.installFromDirs(ctx, opts)
}

// installFromEntries installs only the content items listed in opts.Entries.
func (o *OpenHands) installFromEntries(
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
			written, err := o.installSkillEntry(ctx, opts, entry)
			if err != nil {
				return nil, err
			}

			allFiles = append(allFiles, written...)

		case "mcp":
			mcpPath, err := o.mcpConfigPath(opts)
			if err != nil {
				return nil, err
			}

			if err := fs.InstallMCP(ctx, opts.SourceDir, mcpPath); err != nil {
				return nil, err
			}

		case "hook":
			hooksPath, err := o.hooksPath(opts)
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
func (o *OpenHands) installFromDirs(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	baseDir, skillsDir, err := fs.ResolveDirs(
		opts,
		".openhands/skills",
		".agents/skills",
		o.userHomeFunc,
		o.cwdFunc,
	)
	if err != nil {
		return nil, err
	}

	destDir := filepath.Join(skillsDir, opts.Name)

	if err := o.mkdirAllFunc(destDir, 0o755); err != nil {
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

	mcpPath, mcpErr := o.mcpConfigPath(opts)
	if mcpErr != nil {
		return nil, mcpErr
	}

	if err := fs.InstallMCP(ctx, opts.SourceDir, mcpPath); err != nil {
		return nil, err
	}

	hooksPath, hooksErr := o.hooksPath(opts)
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
func (o *OpenHands) installSkillEntry(
	ctx context.Context,
	opts target.InstallOpts,
	entry target.ContentEntry,
) ([]target.InstalledFile, error) {
	baseDir, skillsDir, err := fs.ResolveDirs(
		opts,
		".openhands/skills",
		".agents/skills",
		o.userHomeFunc,
		o.cwdFunc,
	)
	if err != nil {
		return nil, err
	}

	return fs.InstallSkillEntry(ctx, entry, skillsDir, baseDir, o.mkdirAllFunc)
}

// mcpConfigPath returns the MCP config path for the install scope. Project
// installs use .mcp.json; global installs use ~/.openhands/.mcp.json.
func (o *OpenHands) mcpConfigPath(opts target.InstallOpts) (string, error) {
	if opts.Global {
		home, err := o.userHomeFunc()
		if err != nil {
			return "", fmt.Errorf("home dir: %w", err)
		}

		return filepath.Join(home, ".openhands", ".mcp.json"), nil
	}

	dir := opts.Dir
	if dir == "" {
		cwd, err := o.cwdFunc()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return filepath.Join(dir, ".mcp.json"), nil
}

// hooksPath returns the hooks.json path for the install scope. Project
// installs use hooks.json; global installs use ~/.openhands/hooks.json.
func (o *OpenHands) hooksPath(opts target.InstallOpts) (string, error) {
	if opts.Global {
		home, err := o.userHomeFunc()
		if err != nil {
			return "", fmt.Errorf("home dir: %w", err)
		}

		return filepath.Join(home, ".openhands", "hooks.json"), nil
	}

	dir := opts.Dir
	if dir == "" {
		cwd, err := o.cwdFunc()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return filepath.Join(dir, "hooks.json"), nil
}

// List returns nil; OpenHands does not store managed-plugin metadata.
func (o *OpenHands) List() ([]target.InstalledPlugin, error) {
	return nil, nil
}
