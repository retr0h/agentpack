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

// Package install orchestrates the claudia install pipeline.
package install

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/retr0h/claudia/pkg/archive"
	"github.com/retr0h/claudia/pkg/checksum"
	"github.com/retr0h/claudia/pkg/fetcher"
	"github.com/retr0h/claudia/pkg/metadata"
)

// Options configures an install run.
type Options struct {
	Source    string // local path or URL to .claudia archive
	PluginDir string // ~/.claude/plugins/ (target directory)
}

// Result holds the outcome of a successful install.
type Result struct {
	Name    string
	Version string
	SHA     string
	Dir     string // where the plugin was extracted to
}

// Run installs a single .claudia archive into PluginDir.
//
// The pipeline:
//  1. Fetch the archive (local copy or remote download) into a temp file.
//  2. Extract the archive into a temp directory.
//  3. Verify all checksums.
//  4. Read .claudia/metadata.json from the extraction to obtain plugin identity.
//  5. Move the marketplace directory from temp to PluginDir.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	f, err := fetcher.New(opts.Source)
	if err != nil {
		return nil, fmt.Errorf("fetcher: %w", err)
	}

	// Fetch to a temp file.
	tmpFile, err := os.CreateTemp("", "claudia-install-*.claudia")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpArchive := tmpFile.Name()
	_ = tmpFile.Close()
	defer func() { _ = os.Remove(tmpArchive) }()

	if err := f.Fetch(ctx, opts.Source, tmpArchive); err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Extract to a temp dir for verification.
	tmpDir, err := os.MkdirTemp("", "claudia-install-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := archive.Extract(ctx, tmpArchive, tmpDir); err != nil {
		return nil, fmt.Errorf("extract: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Verify checksums.
	checksumFile, err := findChecksums(tmpDir)
	if err != nil {
		return nil, err
	}

	entries, err := checksum.ReadFile(checksumFile)
	if err != nil {
		return nil, fmt.Errorf("reading checksums: %w", err)
	}

	results, err := checksum.Verify(ctx, tmpDir, entries)
	if err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}

	for _, r := range results {
		if !r.OK {
			return nil, fmt.Errorf("checksum failed for %s: %s", r.Path, r.Err)
		}
	}

	// Read metadata.
	meta, err := findAndReadMetadata(tmpDir)
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Move the marketplace directory to the real PluginDir.
	marketplaceDir, err := findMarketplaceDir(tmpDir)
	if err != nil {
		return nil, err
	}

	destDir := filepath.Join(opts.PluginDir, "marketplaces", meta.Name)
	if err := os.MkdirAll(filepath.Dir(destDir), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir plugin dir: %w", err)
	}

	// Remove existing installation if present.
	if err := os.RemoveAll(destDir); err != nil {
		return nil, fmt.Errorf("remove existing: %w", err)
	}

	if err := os.Rename(marketplaceDir, destDir); err != nil {
		// Rename across devices fails; fall back to copy.
		if err2 := copyDir(ctx, marketplaceDir, destDir); err2 != nil {
			return nil, fmt.Errorf("install: %w", err2)
		}
	}

	return &Result{
		Name:    meta.Name,
		Version: meta.Version,
		SHA:     shortSHA(meta.GitCommitSHA),
		Dir:     destDir,
	}, nil
}

// findChecksums locates the checksums.txt file inside the extracted archive.
func findChecksums(dir string) (string, error) {
	var found string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "checksums.txt" && strings.Contains(path, ".claudia") {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("searching for checksums.txt: %w", err)
	}

	if found == "" {
		return "", fmt.Errorf("checksums.txt not found in archive")
	}

	return found, nil
}

// findAndReadMetadata locates and parses .claudia/metadata.json.
func findAndReadMetadata(dir string) (*metadata.Metadata, error) {
	var found string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "metadata.json" && strings.Contains(path, ".claudia") {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("searching for metadata.json: %w", err)
	}

	if found == "" {
		return nil, fmt.Errorf("metadata.json not found in archive")
	}

	data, err := os.ReadFile(found)
	if err != nil {
		return nil, fmt.Errorf("read metadata.json: %w", err)
	}

	var meta metadata.Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parse metadata.json: %w", err)
	}

	return &meta, nil
}

// findMarketplaceDir finds the top-level marketplace directory in the extracted
// archive (the directory under marketplaces/ that contains the plugin).
func findMarketplaceDir(dir string) (string, error) {
	// Look for a marketplaces/<name> subdirectory.
	marketplacesDir := filepath.Join(dir, "marketplaces")

	entries, err := os.ReadDir(marketplacesDir)
	if err != nil {
		return "", fmt.Errorf("read marketplaces dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			return filepath.Join(marketplacesDir, e.Name()), nil
		}
	}

	return "", fmt.Errorf("no marketplace directory found in archive")
}

// copyDir recursively copies src to dst.
func copyDir(ctx context.Context, src string, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("rel path: %w", err)
		}

		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		return copyFile(path, target)
	})
}

// copyFile copies a single file from src to dst.
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
