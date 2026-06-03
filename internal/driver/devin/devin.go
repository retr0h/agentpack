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

// Package devin is the agentpack target driver for Devin.
// It installs skills into .agents/skills/ (local) or
// ~/.config/devin/skills/ (global), merges MCP server configs into
// .devin/config.json (project), and merges hooks into
// .devin/hooks.v1.json (project).
package devin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/retr0h/agentpack/internal/driver/fs"
	"github.com/retr0h/agentpack/internal/target"
)

// Devin is the target driver for Devin.
type Devin struct {
	configDirFunc func() (string, error)
	cwdFunc       func() (string, error)
	mkdirAllFunc  func(string, os.FileMode) error
}

// New returns a production Devin driver.
func New() *Devin {
	return &Devin{
		configDirFunc: os.UserConfigDir,
		cwdFunc:       os.Getwd,
		mkdirAllFunc:  os.MkdirAll,
	}
}

// Name returns the target identifier.
func (d *Devin) Name() string { return "devin" }

// DisplayName returns the human-readable target name.
func (d *Devin) DisplayName() string { return "Devin" }

// SupportedTypes returns the content types this driver can install.
func (d *Devin) SupportedTypes() []string {
	return []string{"skill", "hook", "mcp"}
}

// Detect returns true if the Devin config directory exists.
func (d *Devin) Detect() bool {
	configDir, err := d.configDirFunc()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(configDir, "devin"))

	return err == nil
}

// Install copies content from opts.SourceDir into the correct locations for
// Devin. When opts.Entries is non-empty the driver installs only the listed
// entries; otherwise it falls back to the legacy directory-walking behaviour
// (skills only). Returns the list of files written.
func (d *Devin) Install(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(opts.Entries) > 0 {
		return d.installFromEntries(ctx, opts)
	}

	return d.installFromDirs(ctx, opts)
}

// installFromEntries installs only the content items listed in opts.Entries.
func (d *Devin) installFromEntries(
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
			written, err := d.installSkillEntry(ctx, opts, entry)
			if err != nil {
				return nil, err
			}

			allFiles = append(allFiles, written...)

		case "mcp":
			mcpPath, err := d.mcpConfigPath(opts)
			if err != nil {
				return nil, err
			}

			if err := fs.InstallMCP(ctx, opts.SourceDir, mcpPath); err != nil {
				return nil, err
			}

		case "hook":
			hooksPath, err := d.hooksPath(opts)
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
func (d *Devin) installFromDirs(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	baseDir, skillsDir, err := d.resolveDirs(opts)
	if err != nil {
		return nil, err
	}

	destDir := filepath.Join(skillsDir, opts.Name)

	if err := d.mkdirAllFunc(destDir, 0o755); err != nil {
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

	mcpPath, mcpErr := d.mcpConfigPath(opts)
	if mcpErr != nil {
		return nil, mcpErr
	}

	if err := fs.InstallMCP(ctx, opts.SourceDir, mcpPath); err != nil {
		return nil, err
	}

	hooksPath, hooksErr := d.hooksPath(opts)
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
func (d *Devin) installSkillEntry(
	ctx context.Context,
	opts target.InstallOpts,
	entry target.ContentEntry,
) ([]target.InstalledFile, error) {
	baseDir, skillsDir, err := d.resolveDirs(opts)
	if err != nil {
		return nil, err
	}

	return fs.InstallSkillEntry(ctx, entry, skillsDir, baseDir, d.mkdirAllFunc)
}

// resolveDirs returns (baseDir, skillsDir) based on whether the install is
// global or local.
func (d *Devin) resolveDirs(opts target.InstallOpts) (string, string, error) {
	if opts.Global {
		configDir, err := d.configDirFunc()
		if err != nil {
			return "", "", fmt.Errorf("config dir: %w", err)
		}

		// Use the parent of configDir as baseDir (e.g. ~ on Unix).
		baseDir := filepath.Dir(configDir)

		return baseDir, filepath.Join(configDir, "devin", "skills"), nil
	}

	dir := opts.Dir
	if dir == "" {
		cwd, err := d.cwdFunc()
		if err != nil {
			return "", "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return dir, filepath.Join(dir, ".agents", "skills"), nil
}

// mcpConfigPath returns the project-level MCP config path. Devin MCP
// configuration lives in .devin/config.json within the project directory.
func (d *Devin) mcpConfigPath(opts target.InstallOpts) (string, error) {
	dir := opts.Dir
	if dir == "" {
		cwd, err := d.cwdFunc()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return filepath.Join(dir, ".devin", "config.json"), nil
}

// hooksPath returns the hooks.v1.json path for the install scope. Devin hooks
// live in .devin/hooks.v1.json within the project directory.
func (d *Devin) hooksPath(opts target.InstallOpts) (string, error) {
	dir := opts.Dir
	if dir == "" {
		cwd, err := d.cwdFunc()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return filepath.Join(dir, ".devin", "hooks.v1.json"), nil
}

// List returns nil; Devin does not store managed-plugin metadata.
func (d *Devin) List() ([]target.InstalledPlugin, error) {
	return nil, nil
}
