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

// Package warp is the agentpack target driver for Warp.
// It installs skills into .agents/skills/ (local) or ~/.warp/skills/
// (global) and merges MCP server configs into .warp/.mcp.json.
package warp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/retr0h/agentpack/internal/driver/fs"
	"github.com/retr0h/agentpack/pkg/target"
)

// Warp is the target driver for Warp.
type Warp struct {
	userHomeFunc func() (string, error)
	cwdFunc      func() (string, error)
	mkdirAllFunc func(string, os.FileMode) error
}

// New returns a production Warp driver.
func New() *Warp {
	return &Warp{
		userHomeFunc: os.UserHomeDir,
		cwdFunc:      os.Getwd,
		mkdirAllFunc: os.MkdirAll,
	}
}

// Name returns the target identifier.
func (w *Warp) Name() string { return "warp" }

// DisplayName returns the human-readable target name.
func (w *Warp) DisplayName() string { return "Warp" }

// SupportedTypes returns the content types this driver can install.
func (w *Warp) SupportedTypes() []string {
	return []string{"skill", "mcp"}
}

// Detect returns true if the Warp config directory exists.
func (w *Warp) Detect() bool {
	home, err := w.userHomeFunc()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(home, ".warp"))

	return err == nil
}

// Install copies content from opts.SourceDir into the correct locations for
// Warp. When opts.Entries is non-empty the driver installs only the listed
// entries; otherwise it falls back to the legacy directory-walking behaviour
// (skills only). Returns the list of files written.
func (w *Warp) Install(
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
func (w *Warp) installFromEntries(
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
			mcpPath, err := w.mcpSettingsPath(opts)
			if err != nil {
				return nil, err
			}

			if err := fs.InstallMCP(ctx, opts.SourceDir, mcpPath); err != nil {
				return nil, err
			}
		}
	}

	return allFiles, nil
}

// installFromDirs walks convention-named directories under opts.SourceDir
// and installs everything found. This is the legacy fallback when no manifest
// entries are provided. Only skills and MCP are handled.
func (w *Warp) installFromDirs(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	baseDir, skillsDir, err := fs.ResolveDirs(
		opts,
		".warp/skills",
		".agents/skills",
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
	if err := fs.CopyTreeIfExists(ctx, skillsSrc, destDir); err != nil {
		return nil, fmt.Errorf("copy skills: %w", err)
	}

	files, err := fs.EnumerateFiles(ctx, destDir, baseDir)
	if err != nil {
		return nil, fmt.Errorf("enumerate installed files: %w", err)
	}

	mcpPath, mcpErr := w.mcpSettingsPath(opts)
	if mcpErr != nil {
		return nil, mcpErr
	}

	if err := fs.InstallMCP(ctx, opts.SourceDir, mcpPath); err != nil {
		return nil, err
	}

	return files, nil
}

// installSkillEntry copies a single skill entry's tree into the skills
// directory.
func (w *Warp) installSkillEntry(
	ctx context.Context,
	opts target.InstallOpts,
	entry target.ContentEntry,
) ([]target.InstalledFile, error) {
	baseDir, skillsDir, err := fs.ResolveDirs(
		opts,
		".warp/skills",
		".agents/skills",
		w.userHomeFunc,
		w.cwdFunc,
	)
	if err != nil {
		return nil, err
	}

	return fs.InstallSkillEntry(ctx, entry, skillsDir, baseDir, w.mkdirAllFunc)
}

// mcpSettingsPath returns the path to .warp/.mcp.json for the install root.
func (w *Warp) mcpSettingsPath(opts target.InstallOpts) (string, error) {
	dir := opts.Dir
	if dir == "" {
		cwd, err := w.cwdFunc()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return filepath.Join(dir, ".warp", ".mcp.json"), nil
}

// List returns nil; Warp does not store managed-plugin metadata.
func (w *Warp) List() ([]target.InstalledPlugin, error) {
	return nil, nil
}
