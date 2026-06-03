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

// Package claudecode is the agentpack target driver for Claude Code.
// It installs content into .claude/ subdirectories (skills, commands,
// agents) relative to the project directory.
package claudecode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/retr0h/agentpack/internal/cli"
	"github.com/retr0h/agentpack/internal/configmerge"
	"github.com/retr0h/agentpack/internal/driver/fs"
	"github.com/retr0h/agentpack/internal/gitutil"
	"github.com/retr0h/agentpack/internal/metadata"
	"github.com/retr0h/agentpack/pkg/target"
)

// ClaudeCode is the target driver for Claude Code.
type ClaudeCode struct {
	userHomeFunc func() (string, error)
	mkdirAllFunc func(string, os.FileMode) error
}

// New returns a production ClaudeCode driver.
func New() *ClaudeCode {
	return &ClaudeCode{
		userHomeFunc: os.UserHomeDir,
		mkdirAllFunc: os.MkdirAll,
	}
}

// Name returns the target identifier.
func (c *ClaudeCode) Name() string { return "claude-code" }

// DisplayName returns the human-readable target name.
func (c *ClaudeCode) DisplayName() string { return "Claude Code" }

// SupportedTypes returns the content types this driver can install.
func (c *ClaudeCode) SupportedTypes() []string {
	return []string{"skill", "command", "hook", "agent", "mcp", "config"}
}

// Detect returns true if the Claude Code config directory exists.
func (c *ClaudeCode) Detect() bool {
	home, err := c.userHomeFunc()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(home, ".claude"))

	return err == nil
}

// entryTypePlural maps singular content-type identifiers to their plural
// directory names under .claude/.
var entryTypePlural = map[string]string{
	"skill":   "skills",
	"command": "commands",
	"agent":   "agents",
}

// Install copies content from opts.SourceDir into .claude/ under opts.Dir.
// When opts.Entries is non-empty the driver installs only the listed entries;
// otherwise it falls back to the legacy directory-walking behaviour.
// Returns the list of files written.
func (c *ClaudeCode) Install(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	root := opts.Dir
	if root == "" {
		home, err := c.userHomeFunc()
		if err != nil {
			return nil, fmt.Errorf("home dir: %w", err)
		}

		root = home
	}

	if len(opts.Entries) > 0 {
		return c.installFromEntries(ctx, opts, root)
	}

	return c.installFromDirs(ctx, opts, root)
}

// installFromEntries installs only the content items listed in opts.Entries.
func (c *ClaudeCode) installFromEntries(
	ctx context.Context,
	opts target.InstallOpts,
	root string,
) ([]target.InstalledFile, error) {
	var allFiles []target.InstalledFile

	settingsPath := filepath.Join(root, ".claude", "settings.json")

	for _, entry := range opts.Entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		switch entry.Type {
		case "skill", "command", "agent":
			plural := entryTypePlural[entry.Type]
			srcDir := entry.Root
			dstDir := filepath.Join(root, ".claude", plural, entry.Name)

			written, err := copyTreeTracked(c.mkdirAllFunc, srcDir, dstDir, root)
			if err != nil {
				return nil, fmt.Errorf("install %s %q: %w", entry.Type, entry.Name, err)
			}

			allFiles = append(allFiles, written...)

		case "mcp":
			if err := c.installMCP(ctx, opts.SourceDir, settingsPath); err != nil {
				return nil, err
			}

		case "hook":
			if err := c.installHooks(ctx, opts.SourceDir, settingsPath, opts.Name); err != nil {
				return nil, err
			}

		case "config":
			if err := c.installSettings(ctx, opts.SourceDir, settingsPath); err != nil {
				return nil, err
			}
		}
	}

	return allFiles, nil
}

// installFromDirs walks convention-named directories under opts.SourceDir
// (skills/, commands/, agents/, mcp/, hooks/, settings/) and installs
// everything found. This is the legacy fallback when no manifest entries
// are provided.
func (c *ClaudeCode) installFromDirs(
	ctx context.Context,
	opts target.InstallOpts,
	root string,
) ([]target.InstalledFile, error) {
	var allFiles []target.InstalledFile

	for _, content := range []string{"skills", "commands", "agents"} {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		srcDir := filepath.Join(opts.SourceDir, content)
		if _, err := os.Stat(srcDir); errors.Is(err, os.ErrNotExist) {
			continue
		}

		dstDir := filepath.Join(root, ".claude", content)

		written, copyErr := copyTreeTracked(c.mkdirAllFunc, srcDir, dstDir, root)
		if copyErr != nil {
			return nil, fmt.Errorf("install %s: %w", content, copyErr)
		}

		allFiles = append(allFiles, written...)
	}

	settingsPath := filepath.Join(root, ".claude", "settings.json")

	if err := c.installMCP(ctx, opts.SourceDir, settingsPath); err != nil {
		return nil, err
	}

	if err := c.installHooks(ctx, opts.SourceDir, settingsPath, opts.Name); err != nil {
		return nil, err
	}

	if err := c.installSettings(ctx, opts.SourceDir, settingsPath); err != nil {
		return nil, err
	}

	return allFiles, nil
}

// installMCP merges all mcp/*.json files from srcDir into settingsPath.
func (c *ClaudeCode) installMCP(ctx context.Context, srcDir, settingsPath string) error {
	mcpDir := filepath.Join(srcDir, "mcp")
	if _, err := os.Stat(mcpDir); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	entries, err := os.ReadDir(mcpDir)
	if err != nil {
		return fmt.Errorf("read mcp dir: %w", err)
	}

	for _, de := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}

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

		if err := configmerge.MergeMCP(settingsPath, name, raw); err != nil {
			return fmt.Errorf("merge mcp %q: %w", name, err)
		}
	}

	return nil
}

// installHooks merges hooks/hooks.json from srcDir into settingsPath.
func (c *ClaudeCode) installHooks(
	_ context.Context,
	srcDir, settingsPath, pluginName string,
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

	if err := configmerge.MergeHooks(settingsPath, pluginName, hooks); err != nil {
		return fmt.Errorf("merge hooks: %w", err)
	}

	return nil
}

// installSettings merges all settings/*.json files from srcDir into settingsPath.
func (c *ClaudeCode) installSettings(ctx context.Context, srcDir, settingsPath string) error {
	settingsDir := filepath.Join(srcDir, "settings")
	if _, err := os.Stat(settingsDir); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	entries, err := os.ReadDir(settingsDir)
	if err != nil {
		return fmt.Errorf("read settings dir: %w", err)
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

		if err := configmerge.MergeSettings(settingsPath, fragment); err != nil {
			return fmt.Errorf("merge settings/%s: %w", de.Name(), err)
		}
	}

	return nil
}

// List scans .claude/ for agentpack-managed content. Currently returns
// entries from the registry rather than filesystem scanning.
func (c *ClaudeCode) List() ([]target.InstalledPlugin, error) {
	home, err := c.userHomeFunc()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}

	marketplacesDir := filepath.Join(home, ".claude", "plugins", "marketplaces")

	dirEntries, err := os.ReadDir(marketplacesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, fmt.Errorf("read marketplaces dir: %w", err)
	}

	var plugins []target.InstalledPlugin

	for _, de := range dirEntries {
		if !de.IsDir() {
			continue
		}

		dir := filepath.Join(marketplacesDir, de.Name())
		metaPath := filepath.Join(dir, ".agentpack", "metadata.json")

		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}

		var meta metadata.Metadata
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}

		plugins = append(plugins, target.InstalledPlugin{
			Name:      meta.Name,
			Version:   meta.Version,
			SHA:       gitutil.ShortSHA(meta.GitCommitSHA),
			Installed: cli.FormatDate(meta.BuildTimestamp),
			Dir:       dir,
			Target:    c.DisplayName(),
		})
	}

	return plugins, nil
}

// copyTreeTracked copies src to dst and returns the files written with
// paths relative to root and SHA256 digests.
func copyTreeTracked(
	mkdirAll func(string, os.FileMode) error,
	src, dst, root string,
) ([]target.InstalledFile, error) {
	var files []target.InstalledFile

	err := filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}

		tgt := filepath.Join(dst, rel)

		if d.IsDir() {
			return mkdirAll(tgt, 0o755)
		}

		if copyErr := fs.CopyFile(path, tgt); copyErr != nil {
			return copyErr
		}

		data, readErr := os.ReadFile(tgt)
		if readErr != nil {
			return readErr
		}

		relToRoot, relErr := filepath.Rel(root, tgt)
		if relErr != nil {
			return relErr
		}

		h := sha256.Sum256(data)
		files = append(files, target.InstalledFile{
			Path:   relToRoot,
			SHA256: hex.EncodeToString(h[:]),
		})

		return nil
	})

	return files, err
}
