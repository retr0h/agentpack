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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/retr0h/agentpack/internal/metadata"
	"github.com/retr0h/agentpack/pkg/target"
)

func init() {
	target.Register(New())
}

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

// Detect returns true if the Claude Code config directory exists.
func (c *ClaudeCode) Detect() bool {
	home, err := c.userHomeFunc()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(home, ".claude"))

	return err == nil
}

// Install copies content from opts.SourceDir into .claude/ under opts.Dir.
// skills/ → .claude/skills/ (recursive, preserves subdirs)
// commands/ → .claude/commands/
// agents/ → .claude/agents/
func (c *ClaudeCode) Install(ctx context.Context, opts target.InstallOpts) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	root := opts.Dir
	if root == "" {
		home, err := c.userHomeFunc()
		if err != nil {
			return fmt.Errorf("home dir: %w", err)
		}

		root = home
	}

	claudeDir := filepath.Join(root, ".claude")

	for _, content := range []string{"skills", "commands", "agents"} {
		if err := ctx.Err(); err != nil {
			return err
		}

		srcDir := filepath.Join(opts.SourceDir, content)
		if _, err := os.Stat(srcDir); os.IsNotExist(err) {
			continue
		}

		dstDir := filepath.Join(claudeDir, content)

		if err := copyTree(c.mkdirAllFunc, srcDir, dstDir); err != nil {
			return fmt.Errorf("install %s: %w", content, err)
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
			continue
		}

		var meta metadata.Metadata
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
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

	return plugins, nil
}

// copyTree recursively copies everything from src into dst.
func copyTree(mkdirAll func(string, os.FileMode) error, src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		tgt := filepath.Join(dst, rel)

		if d.IsDir() {
			return mkdirAll(tgt, 0o755)
		}

		return copyFile(path, tgt)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}

	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}

	return os.WriteFile(dst, data, info.Mode())
}

func shortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}

	return sha
}

func formatDate(ts string) string {
	if idx := strings.IndexByte(ts, 'T'); idx > 0 {
		return ts[:idx]
	}

	return ts
}
