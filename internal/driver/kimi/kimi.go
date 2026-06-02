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

// Package kimi is the agentpack target driver for Kimi Code CLI.
// It installs skills into .agents/skills/ (local) or ~/.config/agents/skills/
// (global) and merges MCP server configs into ~/.kimi/mcp.json.
package kimi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/retr0h/agentpack/internal/configmerge"
	"github.com/retr0h/agentpack/pkg/target"
)

// Kimi is the target driver for Kimi Code CLI.
type Kimi struct {
	userHomeFunc func() (string, error)
	cwdFunc      func() (string, error)
	mkdirAllFunc func(string, os.FileMode) error
}

// New returns a production Kimi driver.
func New() *Kimi {
	return &Kimi{
		userHomeFunc: os.UserHomeDir,
		cwdFunc:      os.Getwd,
		mkdirAllFunc: os.MkdirAll,
	}
}

// Name returns the target identifier.
func (k *Kimi) Name() string { return "kimi-cli" }

// DisplayName returns the human-readable target name.
func (k *Kimi) DisplayName() string { return "Kimi Code CLI" }

// SupportedTypes returns the content types this driver can install.
func (k *Kimi) SupportedTypes() []string {
	return []string{"skill", "mcp"}
}

// Detect returns true if the Kimi config directory exists.
func (k *Kimi) Detect() bool {
	home, err := k.userHomeFunc()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(home, ".kimi"))

	return err == nil
}

// Install copies content from opts.SourceDir into the correct locations for
// Kimi. When opts.Entries is non-empty the driver installs only the listed
// entries; otherwise it falls back to the legacy directory-walking behaviour
// (skills only). Returns the list of files written.
func (k *Kimi) Install(
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
func (k *Kimi) installFromEntries(
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
			mcpPath, err := k.mcpSettingsPath()
			if err != nil {
				return nil, err
			}

			if err := k.installMCP(ctx, opts.SourceDir, mcpPath); err != nil {
				return nil, err
			}
		}
	}

	return allFiles, nil
}

// installFromDirs walks convention-named directories under opts.SourceDir
// and installs everything found. This is the legacy fallback when no manifest
// entries are provided. Only skills and MCP are handled.
func (k *Kimi) installFromDirs(
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
	if err := copyTreeIfExists(ctx, skillsSrc, destDir); err != nil {
		return nil, fmt.Errorf("copy skills: %w", err)
	}

	files, err := enumerateFiles(ctx, destDir, baseDir)
	if err != nil {
		return nil, fmt.Errorf("enumerate installed files: %w", err)
	}

	mcpPath, mcpErr := k.mcpSettingsPath()
	if mcpErr != nil {
		return nil, mcpErr
	}

	if err := k.installMCP(ctx, opts.SourceDir, mcpPath); err != nil {
		return nil, err
	}

	return files, nil
}

// installSkillEntry copies a single skill entry's tree into the skills
// directory.
func (k *Kimi) installSkillEntry(
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

	if err := copyTreeIfExists(ctx, entry.Root, destDir); err != nil {
		return nil, fmt.Errorf("copy skills: %w", err)
	}

	return enumerateFiles(ctx, destDir, baseDir)
}

// resolveDirs returns (baseDir, skillsDir) based on whether the install is
// global or local. Global skills use ~/.config/agents/skills/; local skills
// use .agents/skills/.
func (k *Kimi) resolveDirs(opts target.InstallOpts) (string, string, error) {
	if opts.Global {
		home, err := k.userHomeFunc()
		if err != nil {
			return "", "", fmt.Errorf("home dir: %w", err)
		}

		return home, filepath.Join(home, ".config", "agents", "skills"), nil
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

// mcpSettingsPath returns the path to ~/.kimi/mcp.json.
func (k *Kimi) mcpSettingsPath() (string, error) {
	home, err := k.userHomeFunc()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}

	return filepath.Join(home, ".kimi", "mcp.json"), nil
}

// installMCP merges all mcp/*.json files from srcDir into mcpPath.
func (k *Kimi) installMCP(_ context.Context, srcDir, mcpPath string) error {
	mcpDir := filepath.Join(srcDir, "mcp")
	if _, err := os.Stat(mcpDir); os.IsNotExist(err) {
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

// List returns nil; Kimi does not store managed-plugin metadata.
func (k *Kimi) List() ([]target.InstalledPlugin, error) {
	return nil, nil
}

// enumerateFiles walks destDir and returns InstalledFile entries with paths
// relative to baseDir and SHA256 digests.
func enumerateFiles(ctx context.Context, destDir, baseDir string) ([]target.InstalledFile, error) {
	var files []target.InstalledFile

	err := filepath.WalkDir(destDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		rel, relErr := filepath.Rel(baseDir, path)
		if relErr != nil {
			return relErr
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		h := sha256.Sum256(data)
		files = append(files, target.InstalledFile{
			Path:   rel,
			SHA256: hex.EncodeToString(h[:]),
		})

		return nil
	})

	return files, err
}

func copyTreeIfExists(ctx context.Context, src string, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}

	return filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("rel path: %w", err)
		}

		tgt := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(tgt, 0o755)
		}

		return copyFile(path, tgt)
	})
}

func copyFile(src string, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}

	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}

	if err := os.WriteFile(dst, data, info.Mode()); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}

	return nil
}
