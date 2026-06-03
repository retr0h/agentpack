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

// Package crush is the agentpack target driver for Crush (Charm).
// It installs skills into .agents/skills/ (local) or ~/.config/crush/skills/
// (global), and merges hooks into crush.json in the project root.
package crush

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/retr0h/agentpack/internal/driver/fs"
	"github.com/retr0h/agentpack/pkg/target"
)

// Crush is the target driver for Crush (Charm).
type Crush struct {
	userHomeFunc      func() (string, error)
	userConfigDirFunc func() (string, error)
	cwdFunc           func() (string, error)
	mkdirAllFunc      func(string, os.FileMode) error
}

// New returns a production Crush driver.
func New() *Crush {
	return &Crush{
		userHomeFunc:      os.UserHomeDir,
		userConfigDirFunc: os.UserConfigDir,
		cwdFunc:           os.Getwd,
		mkdirAllFunc:      os.MkdirAll,
	}
}

// Name returns the target identifier.
func (c *Crush) Name() string { return "crush" }

// DisplayName returns the human-readable target name.
func (c *Crush) DisplayName() string { return "Crush" }

// SupportedTypes returns the content types this driver can install.
func (c *Crush) SupportedTypes() []string {
	return []string{"skill", "hook"}
}

// Detect returns true if the Crush config directory exists.
func (c *Crush) Detect() bool {
	configDir, err := c.userConfigDirFunc()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(configDir, "crush"))

	return err == nil
}

// Install copies content from opts.SourceDir into the correct locations for
// Crush. When opts.Entries is non-empty the driver installs only the listed
// entries; otherwise it falls back to the legacy directory-walking behaviour
// (skills and hooks). Returns the list of files written.
func (c *Crush) Install(
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
func (c *Crush) installFromEntries(
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

			if err := fs.InstallHooksJSON(ctx, opts.SourceDir, hooksPath, opts.Name); err != nil {
				return nil, err
			}
		}
	}

	return allFiles, nil
}

// installFromDirs walks convention-named directories under opts.SourceDir
// and installs everything found. This is the legacy fallback when no manifest
// entries are provided. Skills and hooks are handled.
func (c *Crush) installFromDirs(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	baseDir, skillsDir, err := fs.ResolveDirs(
		opts,
		".config/crush/skills",
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
	if err := fs.CopyTreeIfExists(ctx, skillsSrc, destDir); err != nil {
		return nil, fmt.Errorf("copy skills: %w", err)
	}

	files, err := fs.EnumerateFiles(ctx, destDir, baseDir)
	if err != nil {
		return nil, fmt.Errorf("enumerate installed files: %w", err)
	}

	hooksPath, hooksErr := c.hooksPath(opts)
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
func (c *Crush) installSkillEntry(
	ctx context.Context,
	opts target.InstallOpts,
	entry target.ContentEntry,
) ([]target.InstalledFile, error) {
	baseDir, skillsDir, err := fs.ResolveDirs(
		opts,
		".config/crush/skills",
		".agents/skills",
		c.userHomeFunc,
		c.cwdFunc,
	)
	if err != nil {
		return nil, err
	}

	return fs.InstallSkillEntry(ctx, entry, skillsDir, baseDir, c.mkdirAllFunc)
}

// hooksPath returns the crush.json path for the install scope. Crush hooks
// live at crush.json in the project root.
func (c *Crush) hooksPath(opts target.InstallOpts) (string, error) {
	if opts.Global {
		home, err := c.userHomeFunc()
		if err != nil {
			return "", fmt.Errorf("home dir: %w", err)
		}

		return filepath.Join(home, "crush.json"), nil
	}

	dir := opts.Dir
	if dir == "" {
		cwd, err := c.cwdFunc()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return filepath.Join(dir, "crush.json"), nil
}

// List returns nil; Crush does not store managed-plugin metadata.
func (c *Crush) List() ([]target.InstalledPlugin, error) {
	return nil, nil
}
