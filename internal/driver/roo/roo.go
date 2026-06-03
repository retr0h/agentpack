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

// Package roo is the agentpack target driver for Roo Code.
// It installs skills into .agents/skills/ (local) or ~/.roo/skills/
// (global), and merges config values into .roomodes YAML at the project root.
package roo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/retr0h/agentpack/internal/driver/fs"
	"github.com/retr0h/agentpack/pkg/target"
)

// Roo is the target driver for Roo Code.
type Roo struct {
	userHomeFunc func() (string, error)
	cwdFunc      func() (string, error)
	mkdirAllFunc func(string, os.FileMode) error
}

// New returns a production Roo driver.
func New() *Roo {
	return &Roo{
		userHomeFunc: os.UserHomeDir,
		cwdFunc:      os.Getwd,
		mkdirAllFunc: os.MkdirAll,
	}
}

// Name returns the target identifier.
func (r *Roo) Name() string { return "roo" }

// DisplayName returns the human-readable target name.
func (r *Roo) DisplayName() string { return "Roo Code" }

// SupportedTypes returns the content types this driver can install.
func (r *Roo) SupportedTypes() []string {
	return []string{"skill", "config"}
}

// Detect returns true if the Roo config directory exists.
func (r *Roo) Detect() bool {
	home, err := r.userHomeFunc()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(home, ".roo"))

	return err == nil
}

// Install copies content from opts.SourceDir into the correct locations for
// Roo Code. When opts.Entries is non-empty the driver installs only the listed
// skill entries; otherwise it falls back to the legacy directory-walking
// behaviour. Returns the list of files written.
func (r *Roo) Install(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(opts.Entries) > 0 {
		return r.installFromEntries(ctx, opts)
	}

	return r.installFromDirs(ctx, opts)
}

// installFromEntries installs only the content items listed in opts.Entries.
func (r *Roo) installFromEntries(
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
			written, err := r.installSkillEntry(ctx, opts, entry)
			if err != nil {
				return nil, err
			}

			allFiles = append(allFiles, written...)

		case "config":
			roomodesPath, err := r.roomodesPath(opts)
			if err != nil {
				return nil, err
			}

			if err := r.installConfig(opts.SourceDir, roomodesPath); err != nil {
				return nil, err
			}
		}
	}

	return allFiles, nil
}

// installFromDirs walks convention-named directories under opts.SourceDir
// and installs everything found. This is the legacy fallback when no manifest
// entries are provided.
func (r *Roo) installFromDirs(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	baseDir, skillsDir, err := fs.ResolveDirs(
		opts,
		".roo/skills",
		".agents/skills",
		r.userHomeFunc,
		r.cwdFunc,
	)
	if err != nil {
		return nil, err
	}

	destDir := filepath.Join(skillsDir, opts.Name)

	if err := r.mkdirAllFunc(destDir, 0o755); err != nil {
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

	roomodesPath, roomodesErr := r.roomodesPath(opts)
	if roomodesErr != nil {
		return nil, roomodesErr
	}

	if err := r.installConfig(opts.SourceDir, roomodesPath); err != nil {
		return nil, err
	}

	return files, nil
}

// installSkillEntry copies a single skill entry's tree into the skills
// directory.
func (r *Roo) installSkillEntry(
	ctx context.Context,
	opts target.InstallOpts,
	entry target.ContentEntry,
) ([]target.InstalledFile, error) {
	baseDir, skillsDir, err := fs.ResolveDirs(
		opts,
		".roo/skills",
		".agents/skills",
		r.userHomeFunc,
		r.cwdFunc,
	)
	if err != nil {
		return nil, err
	}

	return fs.InstallSkillEntry(ctx, entry, skillsDir, baseDir, r.mkdirAllFunc)
}

// roomodesPath returns the path to .roomodes for the install root.
func (r *Roo) roomodesPath(opts target.InstallOpts) (string, error) {
	dir := opts.Dir
	if dir == "" {
		cwd, err := r.cwdFunc()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return filepath.Join(dir, ".roomodes"), nil
}

// installConfig reads all settings/*.json files from srcDir, parses them, and
// merges each top-level key into the .roomodes YAML config at roomodesPath.
// The config file is created when absent. Existing keys are preserved.
func (r *Roo) installConfig(srcDir, roomodesPath string) error {
	settingsDir := filepath.Join(srcDir, "settings")
	if _, err := os.Stat(settingsDir); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	entries, err := os.ReadDir(settingsDir)
	if err != nil {
		return fmt.Errorf("read settings dir: %w", err)
	}

	for _, de := range entries {
		if de.IsDir() || filepath.Ext(de.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(settingsDir, de.Name()))
		if err != nil {
			return fmt.Errorf("read settings/%s: %w", de.Name(), err)
		}

		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("parse settings/%s: %w", de.Name(), err)
		}

		if err := mergeRoomodes(roomodesPath, raw); err != nil {
			return fmt.Errorf("merge settings/%s: %w", de.Name(), err)
		}
	}

	return nil
}

// mergeRoomodes reads the .roomodes YAML at path, merges all top-level keys
// from patch, and writes back. The file and parent directories are created when
// absent. Existing keys outside patch are preserved.
func mergeRoomodes(path string, patch map[string]any) error {
	cfg, err := readYAMLConfig(path)
	if err != nil {
		return err
	}

	maps.Copy(cfg, patch)

	return writeYAMLConfig(path, cfg)
}

// readYAMLConfig reads and unmarshals the YAML file at path into a map.
// When the file does not exist it returns an empty map.
func readYAMLConfig(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, nil
		}

		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if doc == nil {
		doc = map[string]any{}
	}

	return doc, nil
}

// writeYAMLConfig marshals doc as YAML and writes it to path. Parent
// directories are created as needed.
func writeYAMLConfig(path string, doc map[string]any) error {
	data, err := yaml.Marshal(doc)
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

// List returns nil; Roo does not store managed-plugin metadata.
func (r *Roo) List() ([]target.InstalledPlugin, error) {
	return nil, nil
}
