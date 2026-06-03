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

// Package codex is the agentpack target driver for OpenAI Codex.
// It installs skills into .agents/skills/ (local) or ~/.codex/skills/ (global),
// and merges hooks into .codex/hooks/hooks.json (local) or
// ~/.codex/hooks/hooks.json (global).
package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/retr0h/agentpack/internal/driver"
	"github.com/retr0h/agentpack/internal/target"
)

// Codex is the target driver for OpenAI Codex.
type Codex struct {
	userHomeFunc func() (string, error)
	cwdFunc      func() (string, error)
	mkdirAllFunc func(string, os.FileMode) error
	getenvFunc   func(string) string
}

// New returns a production Codex driver.
func New() *Codex {
	return &Codex{
		userHomeFunc: os.UserHomeDir,
		cwdFunc:      os.Getwd,
		mkdirAllFunc: os.MkdirAll,
		getenvFunc:   os.Getenv,
	}
}

// Name returns the target identifier.
func (c *Codex) Name() string { return "codex" }

// DisplayName returns the human-readable target name.
func (c *Codex) DisplayName() string { return "Codex" }

// SupportedTypes returns the content types this driver can install.
func (c *Codex) SupportedTypes() []string {
	return []string{"skill", "hook", "config"}
}

// Detect returns true if the Codex config directory exists or CODEX_HOME is
// set to an existing path.
func (c *Codex) Detect() bool {
	home, err := c.userHomeFunc()
	if err != nil {
		return false
	}

	if override := c.getenvFunc("CODEX_HOME"); override != "" {
		_, err := os.Stat(override)
		return err == nil
	}

	_, err = os.Stat(filepath.Join(home, ".codex"))

	return err == nil
}

// Install copies content from opts.SourceDir into the correct locations for
// Codex. When opts.Entries is non-empty the driver installs only the listed
// entries; otherwise it falls back to the legacy directory-walking behaviour
// (skills only). Returns the list of files written.
func (c *Codex) Install(
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
func (c *Codex) installFromEntries(
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

		case "config":
			cfgPath, err := c.codexConfigPath(opts)
			if err != nil {
				return nil, err
			}

			if err := c.installConfig(ctx, opts.SourceDir, cfgPath); err != nil {
				return nil, err
			}
		}
	}

	return allFiles, nil
}

// installFromDirs walks convention-named directories under opts.SourceDir
// and installs everything found. This is the legacy fallback when no manifest
// entries are provided. Skills and hooks are handled.
func (c *Codex) installFromDirs(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	baseDir, skillsDir, err := driver.ResolveDirs(
		opts,
		".codex/skills",
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

	cfgPath, cfgErr := c.codexConfigPath(opts)
	if cfgErr != nil {
		return nil, cfgErr
	}

	if err := c.installConfig(ctx, opts.SourceDir, cfgPath); err != nil {
		return nil, err
	}

	return files, nil
}

// installSkillEntry copies a single skill entry's tree into the skills
// directory.
func (c *Codex) installSkillEntry(
	ctx context.Context,
	opts target.InstallOpts,
	entry target.ContentEntry,
) ([]target.InstalledFile, error) {
	baseDir, skillsDir, err := driver.ResolveDirs(
		opts,
		".codex/skills",
		".agents/skills",
		c.userHomeFunc,
		c.cwdFunc,
	)
	if err != nil {
		return nil, err
	}

	return driver.InstallSkillEntry(ctx, entry, skillsDir, baseDir, c.mkdirAllFunc)
}

// hooksPath returns the hooks.json path for the install scope. Project
// installs use .codex/hooks/hooks.json; global installs use
// ~/.codex/hooks/hooks.json.
func (c *Codex) hooksPath(opts target.InstallOpts) (string, error) {
	if opts.Global {
		home, err := c.userHomeFunc()
		if err != nil {
			return "", fmt.Errorf("home dir: %w", err)
		}

		return filepath.Join(home, ".codex", "hooks", "hooks.json"), nil
	}

	dir := opts.Dir
	if dir == "" {
		cwd, err := c.cwdFunc()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return filepath.Join(dir, ".codex", "hooks", "hooks.json"), nil
}

// codexConfigPath returns the path to .codex/config.toml for the install
// scope. Project installs resolve relative to opts.Dir (or cwd); global
// installs resolve under ~/.codex/.
func (c *Codex) codexConfigPath(opts target.InstallOpts) (string, error) {
	if opts.Global {
		home, err := c.userHomeFunc()
		if err != nil {
			return "", fmt.Errorf("home dir: %w", err)
		}

		return filepath.Join(home, ".codex", "config.toml"), nil
	}

	dir := opts.Dir
	if dir == "" {
		cwd, err := c.cwdFunc()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return filepath.Join(dir, ".codex", "config.toml"), nil
}

// installConfig merges all settings/*.json files from srcDir into the TOML
// config at cfgPath. Each JSON file is expected to contain a flat object of
// key-value pairs that are merged at the top level; existing keys not present
// in the fragment are preserved.
func (c *Codex) installConfig(ctx context.Context, srcDir, cfgPath string) error {
	settingsDir := filepath.Join(srcDir, "settings")
	if _, err := os.Stat(settingsDir); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	entries, err := os.ReadDir(settingsDir)
	if err != nil {
		return fmt.Errorf("read settings dir: %w", err)
	}

	// Read existing TOML config (create empty map when absent).
	cfg, err := readTOML(cfgPath)
	if err != nil {
		return err
	}

	for _, de := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}

		if de.IsDir() || filepath.Ext(de.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(settingsDir, de.Name()))
		if err != nil {
			return fmt.Errorf("read settings/%s: %w", de.Name(), err)
		}

		var fragment map[string]any
		if err := json.Unmarshal(data, &fragment); err != nil {
			return fmt.Errorf("parse settings/%s: %w", de.Name(), err)
		}

		maps.Copy(cfg, fragment)
	}

	return writeTOML(cfgPath, cfg)
}

// readTOML reads and unmarshals the TOML file at path into a map.
// When the file does not exist it returns an empty map.
func readTOML(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, nil
		}

		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg map[string]any
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if cfg == nil {
		cfg = map[string]any{}
	}

	return cfg, nil
}

// writeTOML marshals cfg as TOML and writes it to path, creating parent
// directories as needed.
func writeTOML(path string, cfg map[string]any) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

// List returns nil; Codex does not store managed-plugin metadata.
func (c *Codex) List() ([]target.InstalledPlugin, error) {
	return nil, nil
}
