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

// Package goose is the agentpack target driver for Goose.
// It installs skills into .agents/skills/ (local) or ~/.config/goose/skills/
// (global). MCP support is deferred because Goose uses YAML config files.
package goose

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/retr0h/agentpack/pkg/target"
)

func init() {
	target.Register(New())
}

// Goose is the target driver for Goose.
type Goose struct {
	userHomeFunc      func() (string, error)
	userConfigDirFunc func() (string, error)
	cwdFunc           func() (string, error)
	mkdirAllFunc      func(string, os.FileMode) error
}

// New returns a production Goose driver.
func New() *Goose {
	return &Goose{
		userHomeFunc:      os.UserHomeDir,
		userConfigDirFunc: os.UserConfigDir,
		cwdFunc:           os.Getwd,
		mkdirAllFunc:      os.MkdirAll,
	}
}

// Name returns the target identifier.
func (g *Goose) Name() string { return "goose" }

// DisplayName returns the human-readable target name.
func (g *Goose) DisplayName() string { return "Goose" }

// SupportedTypes returns the content types this driver can install.
func (g *Goose) SupportedTypes() []string {
	return []string{"skill"}
}

// Detect returns true if the Goose config directory exists.
func (g *Goose) Detect() bool {
	configDir, err := g.userConfigDirFunc()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(configDir, "goose"))

	return err == nil
}

// Install copies content from opts.SourceDir into the correct locations for
// Goose. When opts.Entries is non-empty the driver installs only the listed
// skill entries; otherwise it falls back to the legacy directory-walking
// behaviour. Returns the list of files written.
func (g *Goose) Install(
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
func (g *Goose) installFromEntries(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	var allFiles []target.InstalledFile

	for _, entry := range opts.Entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if entry.Type == "skill" {
			written, err := g.installSkillEntry(ctx, opts, entry)
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
// entries are provided.
func (g *Goose) installFromDirs(
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
	if err := copyTreeIfExists(ctx, skillsSrc, destDir); err != nil {
		return nil, fmt.Errorf("copy skills: %w", err)
	}

	files, err := enumerateFiles(ctx, destDir, baseDir)
	if err != nil {
		return nil, fmt.Errorf("enumerate installed files: %w", err)
	}

	return files, nil
}

// installSkillEntry copies a single skill entry's tree into the skills
// directory.
func (g *Goose) installSkillEntry(
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

	if err := copyTreeIfExists(ctx, entry.Root, destDir); err != nil {
		return nil, fmt.Errorf("copy skills: %w", err)
	}

	return enumerateFiles(ctx, destDir, baseDir)
}

// resolveDirs returns (baseDir, skillsDir) based on whether the install is
// global or local.
func (g *Goose) resolveDirs(opts target.InstallOpts) (string, string, error) {
	if opts.Global {
		home, err := g.userHomeFunc()
		if err != nil {
			return "", "", fmt.Errorf("home dir: %w", err)
		}

		return home, filepath.Join(home, ".config", "goose", "skills"), nil
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

// List returns nil; Goose does not store managed-plugin metadata.
func (g *Goose) List() ([]target.InstalledPlugin, error) {
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
