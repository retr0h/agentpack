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

// Package gemini is the agentpack target driver for Gemini CLI.
// It installs skills into .agents/skills/ (local) or ~/.gemini/skills/
// (global) and merges MCP server configs into .gemini/settings.json.
package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/retr0h/agentpack/internal/configmerge"
	"github.com/retr0h/agentpack/internal/driver/fs"
	"github.com/retr0h/agentpack/pkg/target"
)

// Gemini is the target driver for Gemini CLI.
type Gemini struct {
	userHomeFunc func() (string, error)
	cwdFunc      func() (string, error)
	mkdirAllFunc func(string, os.FileMode) error
}

// New returns a production Gemini CLI driver.
func New() *Gemini {
	return &Gemini{
		userHomeFunc: os.UserHomeDir,
		cwdFunc:      os.Getwd,
		mkdirAllFunc: os.MkdirAll,
	}
}

// Name returns the target identifier.
func (g *Gemini) Name() string { return "gemini-cli" }

// DisplayName returns the human-readable target name.
func (g *Gemini) DisplayName() string { return "Gemini CLI" }

// SupportedTypes returns the content types this driver can install.
func (g *Gemini) SupportedTypes() []string {
	return []string{"skill", "mcp"}
}

// Detect returns true if the Gemini CLI config directory exists.
func (g *Gemini) Detect() bool {
	home, err := g.userHomeFunc()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(home, ".gemini"))

	return err == nil
}

// Install copies content from opts.SourceDir into the correct locations for
// Gemini CLI. When opts.Entries is non-empty the driver installs only the
// listed entries; otherwise it falls back to the legacy directory-walking
// behaviour (skills only). Returns the list of files written.
func (g *Gemini) Install(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(opts.Entries) > 0 {
		return g.installFromEntries(ctx, opts)
	}

	return g.installFromDirs(ctx, opts)
}

// installFromEntries installs only the content items listed in opts.Entries.
func (g *Gemini) installFromEntries(
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
			written, err := g.installSkillEntry(ctx, opts, entry)
			if err != nil {
				return nil, err
			}

			allFiles = append(allFiles, written...)

		case "mcp":
			mcpPath, err := g.mcpSettingsPath(opts)
			if err != nil {
				return nil, err
			}

			if err := g.installMCP(ctx, opts.SourceDir, mcpPath); err != nil {
				return nil, err
			}
		}
	}

	return allFiles, nil
}

// installFromDirs walks convention-named directories under opts.SourceDir
// and installs everything found. This is the legacy fallback when no manifest
// entries are provided. Only skills and MCP are handled.
func (g *Gemini) installFromDirs(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	baseDir, skillsDir, err := g.resolveDirs(opts)
	if err != nil {
		return nil, err
	}

	destDir := filepath.Join(skillsDir, opts.Name)

	if err := g.mkdirAllFunc(destDir, 0o755); err != nil {
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

	mcpPath, mcpErr := g.mcpSettingsPath(opts)
	if mcpErr != nil {
		return nil, mcpErr
	}

	if err := g.installMCP(ctx, opts.SourceDir, mcpPath); err != nil {
		return nil, err
	}

	return files, nil
}

// installSkillEntry copies a single skill entry's tree into the skills
// directory.
func (g *Gemini) installSkillEntry(
	ctx context.Context,
	opts target.InstallOpts,
	entry target.ContentEntry,
) ([]target.InstalledFile, error) {
	baseDir, skillsDir, err := g.resolveDirs(opts)
	if err != nil {
		return nil, err
	}

	destDir := filepath.Join(skillsDir, entry.Name)

	if err := g.mkdirAllFunc(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir skills dir: %w", err)
	}

	if err := fs.CopyTreeIfExists(ctx, entry.Root, destDir); err != nil {
		return nil, fmt.Errorf("copy skills: %w", err)
	}

	return fs.EnumerateFiles(ctx, destDir, baseDir)
}

// resolveDirs returns (baseDir, skillsDir) based on whether the install is
// global or local.
func (g *Gemini) resolveDirs(opts target.InstallOpts) (string, string, error) {
	if opts.Global {
		home, err := g.userHomeFunc()
		if err != nil {
			return "", "", fmt.Errorf("home dir: %w", err)
		}

		return home, filepath.Join(home, ".gemini", "skills"), nil
	}

	dir := opts.Dir
	if dir == "" {
		cwd, err := g.cwdFunc()
		if err != nil {
			return "", "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return dir, filepath.Join(dir, ".agents", "skills"), nil
}

// mcpSettingsPath returns the path to .gemini/settings.json for the install root.
func (g *Gemini) mcpSettingsPath(opts target.InstallOpts) (string, error) {
	dir := opts.Dir
	if dir == "" {
		cwd, err := g.cwdFunc()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return filepath.Join(dir, ".gemini", "settings.json"), nil
}

// installMCP merges all mcp/*.json files from srcDir into mcpPath.
func (g *Gemini) installMCP(_ context.Context, srcDir, mcpPath string) error {
	mcpDir := filepath.Join(srcDir, "mcp")
	if _, err := os.Stat(mcpDir); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	entries, err := os.ReadDir(mcpDir)
	if err != nil {
		return fmt.Errorf("read mcp dir: %w", err)
	}

	for _, de := range entries {
		if de.IsDir() || filepath.Ext(de.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(mcpDir, de.Name()))
		if err != nil {
			return fmt.Errorf("read mcp/%s: %w", de.Name(), err)
		}

		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("parse mcp/%s: %w", de.Name(), err)
		}

		name, ok := raw["name"].(string)
		if !ok || name == "" {
			return fmt.Errorf("mcp/%s: missing or invalid \"name\" field", de.Name())
		}

		delete(raw, "name")

		if err := configmerge.MergeMCP(mcpPath, name, raw); err != nil {
			return fmt.Errorf("merge mcp %q: %w", name, err)
		}
	}

	return nil
}

// List returns nil; Gemini CLI does not store managed-plugin metadata.
func (g *Gemini) List() ([]target.InstalledPlugin, error) {
	return nil, nil
}
