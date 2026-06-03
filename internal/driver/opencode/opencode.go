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

// Package opencode is the agentpack target driver for OpenCode.
// It installs skills into .agents/skills/ (local) or ~/.config/opencode/skills/
// (global). Agents support uses Markdown+YAML frontmatter which is not yet
// handled, so only skills are supported for now.
package opencode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/retr0h/agentpack/internal/driver/fs"
	"github.com/retr0h/agentpack/pkg/target"
)

// OpenCode is the target driver for OpenCode.
type OpenCode struct {
	userHomeFunc      func() (string, error)
	userConfigDirFunc func() (string, error)
	cwdFunc           func() (string, error)
	mkdirAllFunc      func(string, os.FileMode) error
}

// New returns a production OpenCode driver.
func New() *OpenCode {
	return &OpenCode{
		userHomeFunc:      os.UserHomeDir,
		userConfigDirFunc: os.UserConfigDir,
		cwdFunc:           os.Getwd,
		mkdirAllFunc:      os.MkdirAll,
	}
}

// Name returns the target identifier.
func (o *OpenCode) Name() string { return "opencode" }

// DisplayName returns the human-readable target name.
func (o *OpenCode) DisplayName() string { return "OpenCode" }

// SupportedTypes returns the content types this driver can install.
func (o *OpenCode) SupportedTypes() []string {
	return []string{"skill"}
}

// Detect returns true if the OpenCode config directory exists.
func (o *OpenCode) Detect() bool {
	configDir, err := o.userConfigDirFunc()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(configDir, "opencode"))

	return err == nil
}

// Install copies content from opts.SourceDir into the correct locations for
// OpenCode. When opts.Entries is non-empty the driver installs only the listed
// skill entries; otherwise it falls back to the legacy directory-walking
// behaviour. Returns the list of files written.
func (o *OpenCode) Install(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(opts.Entries) > 0 {
		return o.installFromEntries(ctx, opts)
	}

	return o.installFromDirs(ctx, opts)
}

// installFromEntries installs only the content items listed in opts.Entries.
func (o *OpenCode) installFromEntries(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	var allFiles []target.InstalledFile

	for _, entry := range opts.Entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if entry.Type == "skill" {
			written, err := o.installSkillEntry(ctx, opts, entry)
			if err != nil {
				return nil, err
			}

			allFiles = append(allFiles, written...)
		}
	}

	return allFiles, nil
}

// installFromDirs walks convention-named directories under opts.SourceDir
// and installs everything found. This is the legacy fallback when no manifest
// entries are provided. Only skills are handled.
func (o *OpenCode) installFromDirs(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	baseDir, skillsDir, err := o.resolveDirs(opts)
	if err != nil {
		return nil, err
	}

	destDir := filepath.Join(skillsDir, opts.Name)

	if err := o.mkdirAllFunc(destDir, 0o755); err != nil {
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

	return files, nil
}

// installSkillEntry copies a single skill entry's tree into the skills
// directory.
func (o *OpenCode) installSkillEntry(
	ctx context.Context,
	opts target.InstallOpts,
	entry target.ContentEntry,
) ([]target.InstalledFile, error) {
	baseDir, skillsDir, err := o.resolveDirs(opts)
	if err != nil {
		return nil, err
	}

	destDir := filepath.Join(skillsDir, entry.Name)

	if err := o.mkdirAllFunc(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir skills dir: %w", err)
	}

	if err := fs.CopyTreeIfExists(ctx, entry.Root, destDir); err != nil {
		return nil, fmt.Errorf("copy skills: %w", err)
	}

	return fs.EnumerateFiles(ctx, destDir, baseDir)
}

// resolveDirs returns (baseDir, skillsDir) based on whether the install is
// global or local.
func (o *OpenCode) resolveDirs(opts target.InstallOpts) (string, string, error) {
	if opts.Global {
		home, err := o.userHomeFunc()
		if err != nil {
			return "", "", fmt.Errorf("home dir: %w", err)
		}

		return home, filepath.Join(home, ".config", "opencode", "skills"), nil
	}

	dir := opts.Dir
	if dir == "" {
		cwd, err := o.cwdFunc()
		if err != nil {
			return "", "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return dir, filepath.Join(dir, ".agents", "skills"), nil
}

// List returns nil; OpenCode does not store managed-plugin metadata.
func (o *OpenCode) List() ([]target.InstalledPlugin, error) {
	return nil, nil
}
