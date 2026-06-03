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

// Package cline is the agentpack target driver for Cline.
// It installs skills into .agents/skills/ (local) or ~/.agents/skills/
// (global), copies hook scripts into .clinerules/hooks/ (project), and
// merges MCP server configs into ~/.cline/data/settings/cline_mcp_settings.json.
package cline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/retr0h/agentpack/internal/driver"
	"github.com/retr0h/agentpack/internal/target"
)

// Cline is the target driver for Cline.
type Cline struct {
	userHomeFunc func() (string, error)
	cwdFunc      func() (string, error)
	mkdirAllFunc func(string, os.FileMode) error
}

// New returns a production Cline driver.
func New() *Cline {
	return &Cline{
		userHomeFunc: os.UserHomeDir,
		cwdFunc:      os.Getwd,
		mkdirAllFunc: os.MkdirAll,
	}
}

// Name returns the target identifier.
func (c *Cline) Name() string { return "cline" }

// DisplayName returns the human-readable target name.
func (c *Cline) DisplayName() string { return "Cline" }

// SupportedTypes returns the content types this driver can install.
func (c *Cline) SupportedTypes() []string {
	return []string{"skill", "hook", "mcp"}
}

// Detect returns true if the Cline config directory exists.
func (c *Cline) Detect() bool {
	home, err := c.userHomeFunc()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(home, ".cline"))

	return err == nil
}

// Install copies content from opts.SourceDir into the correct locations for
// Cline. When opts.Entries is non-empty the driver installs only the listed
// entries; otherwise it falls back to the legacy directory-walking behaviour
// (skills only). Returns the list of files written.
func (c *Cline) Install(
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
func (c *Cline) installFromEntries(
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

		case "mcp":
			mcpPath, err := c.mcpConfigPath()
			if err != nil {
				return nil, err
			}

			if err := driver.InstallMCP(ctx, opts.SourceDir, mcpPath); err != nil {
				return nil, err
			}

		case "hook":
			hooksDir, err := c.hooksDir(opts)
			if err != nil {
				return nil, err
			}

			if err := c.installHooks(ctx, entry.Root, hooksDir); err != nil {
				return nil, err
			}
		}
	}

	return allFiles, nil
}

// installFromDirs walks convention-named directories under opts.SourceDir
// and installs everything found. This is the legacy fallback when no manifest
// entries are provided. Skills, MCP, and hooks are handled.
func (c *Cline) installFromDirs(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	baseDir, skillsDir, err := driver.ResolveDirs(
		opts,
		".agents/skills",
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

	mcpPath, mcpErr := c.mcpConfigPath()
	if mcpErr != nil {
		return nil, mcpErr
	}

	if err := driver.InstallMCP(ctx, opts.SourceDir, mcpPath); err != nil {
		return nil, err
	}

	hooksDir, hooksDirErr := c.hooksDir(opts)
	if hooksDirErr != nil {
		return nil, hooksDirErr
	}

	hooksSrc := filepath.Join(opts.SourceDir, "hooks")
	if err := c.installHooks(ctx, hooksSrc, hooksDir); err != nil {
		return nil, err
	}

	return files, nil
}

// installSkillEntry copies a single skill entry's tree into the skills
// directory.
func (c *Cline) installSkillEntry(
	ctx context.Context,
	opts target.InstallOpts,
	entry target.ContentEntry,
) ([]target.InstalledFile, error) {
	baseDir, skillsDir, err := driver.ResolveDirs(
		opts,
		".agents/skills",
		".agents/skills",
		c.userHomeFunc,
		c.cwdFunc,
	)
	if err != nil {
		return nil, err
	}

	return driver.InstallSkillEntry(ctx, entry, skillsDir, baseDir, c.mkdirAllFunc)
}

// mcpConfigPath returns the global MCP config path for Cline.
func (c *Cline) mcpConfigPath() (string, error) {
	home, err := c.userHomeFunc()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}

	return filepath.Join(home, ".cline", "data", "settings", "cline_mcp_settings.json"), nil
}

// hooksDir returns the hooks directory path for Cline. Cline hooks are
// executable scripts stored in .clinerules/hooks/ (project-local).
func (c *Cline) hooksDir(opts target.InstallOpts) (string, error) {
	dir := opts.Dir
	if dir == "" {
		cwd, err := c.cwdFunc()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return filepath.Join(dir, ".clinerules", "hooks"), nil
}

// installHooks copies executable hook scripts from srcDir into hooksDir.
// Cline hooks are executable scripts (not JSON) that receive JSON on stdin
// and return JSON on stdout. Each file is copied and made executable.
func (c *Cline) installHooks(_ context.Context, srcDir, hooksDir string) error {
	if _, err := os.Stat(srcDir); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("read hooks dir: %w", err)
	}

	if err := c.mkdirAllFunc(hooksDir, 0o755); err != nil {
		return fmt.Errorf("mkdir hooks dir: %w", err)
	}

	for _, de := range entries {
		if de.IsDir() {
			continue
		}

		srcPath := filepath.Join(srcDir, de.Name())
		dstPath := filepath.Join(hooksDir, de.Name())

		data, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("read hook %s: %w", de.Name(), err)
		}

		if err := os.WriteFile(dstPath, data, 0o755); err != nil {
			return fmt.Errorf("write hook %s: %w", de.Name(), err)
		}
	}

	return nil
}

// List returns nil; Cline does not store managed-plugin metadata.
func (c *Cline) List() ([]target.InstalledPlugin, error) {
	return nil, nil
}
