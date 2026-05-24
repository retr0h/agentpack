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

// Package claudecode is the agentpack target driver for Claude Code. It
// installs plugins into ~/.claude/plugins/marketplaces/{name}/ and lists
// installed plugins by scanning that directory tree.
package claudecode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/retr0h/agentpack/pkg/manifest"
	"github.com/retr0h/agentpack/pkg/metadata"
	"github.com/retr0h/agentpack/pkg/plugin"
	"github.com/retr0h/agentpack/pkg/target"
)

func init() {
	target.Register(New())
}

// ClaudeCode is the target driver for Claude Code. It satisfies
// target.Target. All OS-level calls go through swappable function fields so
// tests can inject failures without touching global state.
type ClaudeCode struct {
	userHomeFunc  func() (string, error)
	renameFunc    func(string, string) error
	mkdirAllFunc  func(string, os.FileMode) error
	removeAllFunc func(string) error
}

// New returns a production ClaudeCode driver wired to the real OS functions.
func New() *ClaudeCode {
	return &ClaudeCode{
		userHomeFunc:  os.UserHomeDir,
		renameFunc:    os.Rename,
		mkdirAllFunc:  os.MkdirAll,
		removeAllFunc: os.RemoveAll,
	}
}

// Name returns the agent identifier.
func (c *ClaudeCode) Name() string { return "claude-code" }

// DisplayName returns the human-readable agent name.
func (c *ClaudeCode) DisplayName() string { return "Claude Code" }

// Detect returns true when ~/.claude/ exists, indicating Claude Code is
// installed on the current system.
func (c *ClaudeCode) Detect() bool {
	home, err := c.userHomeFunc()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(home, ".claude"))

	return err == nil
}

// Install lays out the plugin from opts.SourceDir into the Claude Code
// marketplace directory structure under ~/.claude/plugins/marketplaces/.
//
// It:
//  1. Atomically moves (or copies) SourceDir to the marketplace destination.
//  2. Generates .claude-plugin/marketplace.json and plugin.json inside the
//     destination.
func (c *ClaudeCode) Install(ctx context.Context, opts target.InstallOpts) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	home, err := c.userHomeFunc()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}

	destDir := filepath.Join(home, ".claude", "plugins", "marketplaces", opts.Name)

	if err := c.mkdirAllFunc(filepath.Dir(destDir), 0o755); err != nil {
		return fmt.Errorf("mkdir plugin dir: %w", err)
	}

	// Remove any existing installation.
	if err := c.removeAllFunc(destDir); err != nil {
		return fmt.Errorf("remove existing: %w", err)
	}

	// Attempt atomic rename; fall back to recursive copy on cross-device move.
	if err := c.renameFunc(opts.SourceDir, destDir); err != nil {
		if err2 := copyDir(ctx, opts.SourceDir, destDir); err2 != nil {
			return fmt.Errorf("install: %w", err2)
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	// Generate and write Claude Code plugin descriptors.
	return c.writeDescriptors(destDir, opts)
}

// List scans ~/.claude/plugins/marketplaces/*/ for agentpack-managed plugins.
// A directory is considered an agentpack plugin when it contains a
// .agentpack/metadata.json file.
func (c *ClaudeCode) List() ([]target.InstalledPlugin, error) {
	home, err := c.userHomeFunc()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}

	marketplacesDir := filepath.Join(home, ".claude", "plugins", "marketplaces")

	dirEntries, err := os.ReadDir(marketplacesDir)
	if err != nil {
		if os.IsNotExist(err) {
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
			// Not an agentpack plugin — skip silently.
			continue
		}

		var meta metadata.Metadata
		if err := json.Unmarshal(data, &meta); err != nil {
			return nil, fmt.Errorf("parse metadata.json in %s: %w", dir, err)
		}

		plugins = append(plugins, target.InstalledPlugin{
			Name:      meta.Name,
			Version:   meta.Version,
			SHA:       shortSHA(meta.GitCommitSHA),
			Installed: formatDate(meta.BuildTimestamp),
			Dir:       dir,
			Target:    c.DisplayName(),
		})
	}

	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].Name < plugins[j].Name
	})

	return plugins, nil
}

// --------------------------------------------------------------------------
// private helpers
// --------------------------------------------------------------------------

// writeDescriptors generates marketplace.json and plugin.json from the
// manifest embedded in the archive (agentpack.yaml) and writes them into
// destDir under .claude-plugin/.
func (c *ClaudeCode) writeDescriptors(destDir string, opts target.InstallOpts) error {
	p, err := readManifestPlugin(destDir, opts)
	if err != nil {
		// If no agentpack.yaml present, synthesize a minimal plugin from
		// metadata.
		p = synthPlugin(opts)
	}

	descDir := filepath.Join(destDir, ".claude-plugin")
	if err := os.MkdirAll(descDir, 0o755); err != nil {
		return fmt.Errorf("mkdir .claude-plugin: %w", err)
	}

	marketplaceJSON, err := plugin.GenerateMarketplace(p)
	if err != nil {
		return fmt.Errorf("generate marketplace.json: %w", err)
	}

	if err := os.WriteFile(filepath.Join(descDir, "marketplace.json"), marketplaceJSON, 0o644); err != nil {
		return fmt.Errorf("write marketplace.json: %w", err)
	}

	commandPaths := collectCommandPaths(destDir)

	pluginJSON, err := plugin.GeneratePlugin(p, commandPaths)
	if err != nil {
		return fmt.Errorf("generate plugin.json: %w", err)
	}

	if err := os.WriteFile(filepath.Join(descDir, "plugin.json"), pluginJSON, 0o644); err != nil {
		return fmt.Errorf("write plugin.json: %w", err)
	}

	return nil
}

// readManifestPlugin reads .agentpack/agentpack.yaml from the installed dir
// and returns the first plugin entry.
func readManifestPlugin(dir string, opts target.InstallOpts) (manifest.Plugin, error) {
	yamlPath := filepath.Join(dir, ".agentpack", "agentpack.yaml")

	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return manifest.Plugin{}, fmt.Errorf("read agentpack.yaml: %w", err)
	}

	var m manifest.Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return manifest.Plugin{}, fmt.Errorf("parse agentpack.yaml: %w", err)
	}

	plugins := manifest.Normalize(&m)
	if len(plugins) == 0 {
		return manifest.Plugin{}, fmt.Errorf("no plugins in agentpack.yaml")
	}

	p := plugins[0]

	// Metadata is the authoritative source for name and version.
	if opts.Name != "" {
		p.Name = opts.Name
	}

	if opts.Version != "" {
		p.Version = opts.Version
	}

	return p, nil
}

// synthPlugin builds a minimal manifest.Plugin from InstallOpts when no
// agentpack.yaml is embedded in the archive.
func synthPlugin(opts target.InstallOpts) manifest.Plugin {
	p := manifest.Plugin{
		Name:    opts.Name,
		Version: opts.Version,
	}

	if opts.Meta != nil {
		if p.Name == "" {
			p.Name = opts.Meta.Name
		}

		if p.Version == "" {
			p.Version = opts.Meta.Version
		}
	}

	return p
}

// collectCommandPaths returns the relative paths of all files under
// dir/commands/ (used to populate plugin.json Commands[]).
func collectCommandPaths(dir string) []string {
	commandsDir := filepath.Join(dir, "commands")

	entries, err := os.ReadDir(commandsDir)
	if err != nil {
		return nil
	}

	var paths []string

	for _, e := range entries {
		if !e.IsDir() {
			paths = append(paths, filepath.Join("commands", e.Name()))
		}
	}

	return paths
}

// copyDir recursively copies src to dst, respecting ctx cancellation.
func copyDir(ctx context.Context, src string, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
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

// copyFile copies a single file preserving its permission bits.
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

// shortSHA returns the first 7 characters of a git commit SHA.
func shortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}

	return sha
}

// formatDate trims an RFC3339 timestamp to its date portion (YYYY-MM-DD).
func formatDate(ts string) string {
	if idx := strings.IndexByte(ts, 'T'); idx > 0 {
		return ts[:idx]
	}

	return ts
}
