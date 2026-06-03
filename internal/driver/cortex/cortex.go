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

// Package cortex is the agentpack target driver for Cortex Code (Snowflake).
// It installs skills into .agents/skills/ (local) or
// ~/.snowflake/cortex/skills/ (global), merges hooks into
// ~/.snowflake/cortex/hooks.json, and merges MCP server configs into
// ~/.snowflake/cortex/mcp.json.
package cortex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/retr0h/agentpack/internal/driver"
	"github.com/retr0h/agentpack/internal/target"
)

// Cortex is the target driver for Cortex Code (Snowflake).
type Cortex struct {
	userHomeFunc func() (string, error)
	cwdFunc      func() (string, error)
	mkdirAllFunc func(string, os.FileMode) error
}

// New returns a production Cortex driver.
func New() *Cortex {
	return &Cortex{
		userHomeFunc: os.UserHomeDir,
		cwdFunc:      os.Getwd,
		mkdirAllFunc: os.MkdirAll,
	}
}

// Name returns the target identifier.
func (c *Cortex) Name() string { return "cortex" }

// DisplayName returns the human-readable target name.
func (c *Cortex) DisplayName() string { return "Cortex Code" }

// SupportedTypes returns the content types this driver can install.
func (c *Cortex) SupportedTypes() []string {
	return []string{"skill", "hook", "mcp"}
}

// Detect returns true if the Cortex config directory exists.
func (c *Cortex) Detect() bool {
	home, err := c.userHomeFunc()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(home, ".snowflake", "cortex"))

	return err == nil
}

// Install copies content from opts.SourceDir into the correct locations for
// Cortex. When opts.Entries is non-empty the driver installs only the listed
// entries; otherwise it falls back to the legacy directory-walking behaviour
// (skills, hooks, and MCP). Returns the list of files written.
func (c *Cortex) Install(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(opts.Entries) > 0 {
		return c.installFromEntries(ctx, opts)
	}

	return c.installFromDirs(ctx, opts)
}

// installFromEntries installs only the content items listed in opts.Entries.
func (c *Cortex) installFromEntries(
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
			written, err := c.installSkillEntry(ctx, opts, entry)
			if err != nil {
				return nil, err
			}

			allFiles = append(allFiles, written...)

		case "hook":
			hooksPath, err := c.hooksPath(opts)
			if err != nil {
				return nil, err
			}

			if err := driver.InstallHooksJSON(ctx, opts.SourceDir, hooksPath, opts.Name); err != nil {
				return nil, err
			}

		case "mcp":
			mcpPath, err := c.mcpConfigPath(opts)
			if err != nil {
				return nil, err
			}

			if err := driver.InstallMCP(ctx, opts.SourceDir, mcpPath); err != nil {
				return nil, err
			}
		}
	}

	return allFiles, nil
}

// installFromDirs walks convention-named directories under opts.SourceDir
// and installs everything found. This is the legacy fallback when no manifest
// entries are provided. Skills, hooks, and MCP are handled.
func (c *Cortex) installFromDirs(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	baseDir, skillsDir, err := driver.ResolveDirs(
		opts,
		".snowflake/cortex/skills",
		".agents/skills",
		c.userHomeFunc,
		c.cwdFunc,
	)
	if err != nil {
		return nil, err
	}

	destDir := filepath.Join(skillsDir, opts.Name)

	if err := c.mkdirAllFunc(destDir, 0o755); err != nil {
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

	hooksPath, hooksErr := c.hooksPath(opts)
	if hooksErr != nil {
		return nil, hooksErr
	}

	if err := driver.InstallHooksJSON(ctx, opts.SourceDir, hooksPath, opts.Name); err != nil {
		return nil, err
	}

	mcpPath, mcpErr := c.mcpConfigPath(opts)
	if mcpErr != nil {
		return nil, mcpErr
	}

	if err := driver.InstallMCP(ctx, opts.SourceDir, mcpPath); err != nil {
		return nil, err
	}

	return files, nil
}

// installSkillEntry copies a single skill entry's tree into the skills
// directory.
func (c *Cortex) installSkillEntry(
	ctx context.Context,
	opts target.InstallOpts,
	entry target.ContentEntry,
) ([]target.InstalledFile, error) {
	baseDir, skillsDir, err := driver.ResolveDirs(
		opts,
		".snowflake/cortex/skills",
		".agents/skills",
		c.userHomeFunc,
		c.cwdFunc,
	)
	if err != nil {
		return nil, err
	}

	return driver.InstallSkillEntry(ctx, entry, skillsDir, baseDir, c.mkdirAllFunc)
}

// hooksPath returns the hooks.json path for the install scope. Both project
// and global installs use ~/.snowflake/cortex/hooks.json.
func (c *Cortex) hooksPath(opts target.InstallOpts) (string, error) {
	if opts.Global {
		home, err := c.userHomeFunc()
		if err != nil {
			return "", fmt.Errorf("home dir: %w", err)
		}

		return filepath.Join(home, ".snowflake", "cortex", "hooks.json"), nil
	}

	dir := opts.Dir
	if dir == "" {
		cwd, err := c.cwdFunc()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return filepath.Join(dir, ".snowflake", "cortex", "hooks.json"), nil
}

// mcpConfigPath returns the mcp.json path for the install scope.
func (c *Cortex) mcpConfigPath(opts target.InstallOpts) (string, error) {
	if opts.Global {
		home, err := c.userHomeFunc()
		if err != nil {
			return "", fmt.Errorf("home dir: %w", err)
		}

		return filepath.Join(home, ".snowflake", "cortex", "mcp.json"), nil
	}

	dir := opts.Dir
	if dir == "" {
		cwd, err := c.cwdFunc()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return filepath.Join(dir, ".snowflake", "cortex", "mcp.json"), nil
}

// List returns nil; Cortex does not store managed-plugin metadata.
func (c *Cortex) List() ([]target.InstalledPlugin, error) {
	return nil, nil
}
