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

// Package kiro is the agentpack target driver for Kiro CLI.
// It installs skills into .agents/skills/ (local) or ~/.kiro/skills/
// (global), merges MCP server configs into .kiro/mcp.json (project-level),
// and merges hooks into .kiro/hooks.json (project-level).
package kiro

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

// Kiro is the target driver for Kiro CLI.
type Kiro struct {
	userHomeFunc func() (string, error)
	cwdFunc      func() (string, error)
	mkdirAllFunc func(string, os.FileMode) error
}

// New returns a production Kiro driver.
func New() *Kiro {
	return &Kiro{
		userHomeFunc: os.UserHomeDir,
		cwdFunc:      os.Getwd,
		mkdirAllFunc: os.MkdirAll,
	}
}

// Name returns the target identifier.
func (k *Kiro) Name() string { return "kiro-cli" }

// DisplayName returns the human-readable target name.
func (k *Kiro) DisplayName() string { return "Kiro CLI" }

// SupportedTypes returns the content types this driver can install.
func (k *Kiro) SupportedTypes() []string {
	return []string{"skill", "mcp", "hook"}
}

// Detect returns true if the Kiro config directory exists.
func (k *Kiro) Detect() bool {
	home, err := k.userHomeFunc()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(home, ".kiro"))

	return err == nil
}

// Install copies content from opts.SourceDir into the correct locations for
// Kiro. When opts.Entries is non-empty the driver installs only the listed
// entries; otherwise it falls back to the legacy directory-walking behaviour
// (skills, MCP, and hooks). Returns the list of files written.
func (k *Kiro) Install(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(opts.Entries) > 0 {
		return k.installFromEntries(ctx, opts)
	}

	return k.installFromDirs(ctx, opts)
}

// installFromEntries installs only the content items listed in opts.Entries.
func (k *Kiro) installFromEntries(
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
			written, err := k.installSkillEntry(ctx, opts, entry)
			if err != nil {
				return nil, err
			}

			allFiles = append(allFiles, written...)

		case "mcp":
			mcpPath, err := k.mcpConfigPath(opts)
			if err != nil {
				return nil, err
			}

			if err := k.installMCP(ctx, opts.SourceDir, mcpPath); err != nil {
				return nil, err
			}

		case "hook":
			hooksPath, err := k.hooksPath(opts)
			if err != nil {
				return nil, err
			}

			if err := k.installHooks(ctx, opts.SourceDir, hooksPath, opts.Name); err != nil {
				return nil, err
			}
		}
	}

	return allFiles, nil
}

// installFromDirs walks convention-named directories under opts.SourceDir
// and installs everything found. This is the legacy fallback when no manifest
// entries are provided. Skills, MCP, and hooks are handled.
func (k *Kiro) installFromDirs(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	baseDir, skillsDir, err := k.resolveDirs(opts)
	if err != nil {
		return nil, err
	}

	destDir := filepath.Join(skillsDir, opts.Name)

	if err := k.mkdirAllFunc(destDir, 0o755); err != nil {
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

	mcpPath, mcpErr := k.mcpConfigPath(opts)
	if mcpErr != nil {
		return nil, mcpErr
	}

	if err := k.installMCP(ctx, opts.SourceDir, mcpPath); err != nil {
		return nil, err
	}

	hooksPath, hooksErr := k.hooksPath(opts)
	if hooksErr != nil {
		return nil, hooksErr
	}

	if err := k.installHooks(ctx, opts.SourceDir, hooksPath, opts.Name); err != nil {
		return nil, err
	}

	return files, nil
}

// installSkillEntry copies a single skill entry's tree into the skills
// directory.
func (k *Kiro) installSkillEntry(
	ctx context.Context,
	opts target.InstallOpts,
	entry target.ContentEntry,
) ([]target.InstalledFile, error) {
	baseDir, skillsDir, err := k.resolveDirs(opts)
	if err != nil {
		return nil, err
	}

	destDir := filepath.Join(skillsDir, entry.Name)

	if err := k.mkdirAllFunc(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir skills dir: %w", err)
	}

	if err := fs.CopyTreeIfExists(ctx, entry.Root, destDir); err != nil {
		return nil, fmt.Errorf("copy skills: %w", err)
	}

	return fs.EnumerateFiles(ctx, destDir, baseDir)
}

// resolveDirs returns (baseDir, skillsDir) based on whether the install is
// global or local.
func (k *Kiro) resolveDirs(opts target.InstallOpts) (string, string, error) {
	if opts.Global {
		home, err := k.userHomeFunc()
		if err != nil {
			return "", "", fmt.Errorf("home dir: %w", err)
		}

		return home, filepath.Join(home, ".kiro", "skills"), nil
	}

	dir := opts.Dir
	if dir == "" {
		cwd, err := k.cwdFunc()
		if err != nil {
			return "", "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return dir, filepath.Join(dir, ".agents", "skills"), nil
}

// mcpConfigPath returns the project-level MCP config path. Kiro MCP config
// lives at .kiro/mcp.json within the project directory.
func (k *Kiro) mcpConfigPath(opts target.InstallOpts) (string, error) {
	if opts.Global {
		home, err := k.userHomeFunc()
		if err != nil {
			return "", fmt.Errorf("home dir: %w", err)
		}

		return filepath.Join(home, ".kiro", "mcp.json"), nil
	}

	dir := opts.Dir
	if dir == "" {
		cwd, err := k.cwdFunc()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return filepath.Join(dir, ".kiro", "mcp.json"), nil
}

// hooksPath returns the hooks.json path for the install scope. Project
// installs use .kiro/hooks.json; global installs use ~/.kiro/hooks.json.
func (k *Kiro) hooksPath(opts target.InstallOpts) (string, error) {
	if opts.Global {
		home, err := k.userHomeFunc()
		if err != nil {
			return "", fmt.Errorf("home dir: %w", err)
		}

		return filepath.Join(home, ".kiro", "hooks.json"), nil
	}

	dir := opts.Dir
	if dir == "" {
		cwd, err := k.cwdFunc()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return filepath.Join(dir, ".kiro", "hooks.json"), nil
}

// installMCP merges all mcp/*.json files from srcDir into mcpPath.
func (k *Kiro) installMCP(_ context.Context, srcDir, mcpPath string) error {
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

// installHooks merges hooks/hooks.json from srcDir into hooksPath.
func (k *Kiro) installHooks(
	_ context.Context,
	srcDir, hooksPath, pluginName string,
) error {
	hooksFile := filepath.Join(srcDir, "hooks", "hooks.json")
	if _, err := os.Stat(hooksFile); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	data, err := os.ReadFile(hooksFile)
	if err != nil {
		return fmt.Errorf("read hooks/hooks.json: %w", err)
	}

	var hooks map[string]any
	if err := json.Unmarshal(data, &hooks); err != nil {
		return fmt.Errorf("parse hooks/hooks.json: %w", err)
	}

	if err := configmerge.MergeHooks(hooksPath, pluginName, hooks); err != nil {
		return fmt.Errorf("merge hooks: %w", err)
	}

	return nil
}

// List returns nil; Kiro does not store managed-plugin metadata.
func (k *Kiro) List() ([]target.InstalledPlugin, error) {
	return nil, nil
}
