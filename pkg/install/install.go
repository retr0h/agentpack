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

// Package install orchestrates the agentpack install pipeline.
package install

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/retr0h/agentpack/pkg/archive"
	"github.com/retr0h/agentpack/pkg/checksum"
	"github.com/retr0h/agentpack/pkg/fetcher"
	"github.com/retr0h/agentpack/pkg/metadata"
	"github.com/retr0h/agentpack/pkg/target"
)

// Swappable OS functions for testing.
var (
	// osCreateTemp is a swappable wrapper so tests can inject temp-file
	// creation failures.
	osCreateTemp = os.CreateTemp
	osMkdirTemp  = os.MkdirTemp
)

// Options configures an install run.
type Options struct {
	// Source is the local path or URL to the .agentpack archive.
	Source string

	// Targets is the list of agent targets to install into. When nil or empty
	// the global target registry is consulted and only detected targets are
	// used.
	Targets []target.Target
}

// Result holds the outcome of a successful install.
type Result struct {
	Name    string
	Version string
	SHA     string
	// Dirs maps target display-name → installed directory.
	Dirs map[string]string
}

// Run installs a single .agentpack archive into every detected target.
//
// The pipeline:
//  1. Fetch the archive (local copy or remote download) into a temp file.
//  2. Extract the archive into a temp directory.
//  3. Verify all checksums.
//  4. Read .agentpack/metadata.json to obtain plugin identity.
//  5. Call target.Install for each detected (or provided) agent target.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	f, err := fetcher.New(opts.Source)
	if err != nil {
		return nil, fmt.Errorf("fetcher: %w", err)
	}

	// Fetch to a temp file.
	tmpFile, err := osCreateTemp("", "agentpack-install-*.agentpack")
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
	tmpDir, err := osMkdirTemp("", "agentpack-install-*")
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

	checksumEntries, err := checksum.ReadFile(checksumFile)
	if err != nil {
		return nil, fmt.Errorf("reading checksums: %w", err)
	}

	verifyResults, err := checksum.Verify(ctx, tmpDir, checksumEntries)
	if err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}

	for _, r := range verifyResults {
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

	// Resolve the list of targets to install into.
	targets := opts.Targets
	if len(targets) == 0 {
		targets = target.Detected()
	}

	installOpts := target.InstallOpts{
		Name:    meta.Name,
		Version: meta.Version,
		Meta:    meta,
	}

	dirs := make(map[string]string)

	for _, tgt := range targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Each target driver may consume (rename) its SourceDir. To support
		// multiple targets we always supply a fresh copy of the extracted
		// archive for each target.
		srcDir, err := copyToTemp(ctx, tmpDir)
		if err != nil {
			return nil, fmt.Errorf("prepare source for %s: %w", tgt.Name(), err)
		}

		installOpts.SourceDir = srcDir

		if installErr := tgt.Install(ctx, installOpts); installErr != nil {
			_ = os.RemoveAll(srcDir)

			return nil, fmt.Errorf("install to %s: %w", tgt.Name(), installErr)
		}

		dirs[tgt.DisplayName()] = srcDir
	}

	return &Result{
		Name:    meta.Name,
		Version: meta.Version,
		SHA:     shortSHA(meta.GitCommitSHA),
		Dirs:    dirs,
	}, nil
}

// copyToTemp makes a fresh copy of src into a new temp directory and returns
// the path to the new directory.
func copyToTemp(ctx context.Context, src string) (string, error) {
	dst, err := osMkdirTemp("", "agentpack-target-*")
	if err != nil {
		return "", fmt.Errorf("create target temp dir: %w", err)
	}

	if copyErr := copyDir(ctx, src, dst); copyErr != nil {
		_ = os.RemoveAll(dst)

		return "", fmt.Errorf("copy to target dir: %w", copyErr)
	}

	return dst, nil
}

// findChecksums locates the checksums.txt file inside the extracted archive.
// The generic archive layout places it at .agentpack/checksums.txt.
func findChecksums(dir string) (string, error) {
	var found string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && d.Name() == "checksums.txt" && strings.Contains(path, ".agentpack") {
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

// findAndReadMetadata locates and parses .agentpack/metadata.json.
func findAndReadMetadata(dir string) (*metadata.Metadata, error) {
	var found string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && d.Name() == "metadata.json" && strings.Contains(path, ".agentpack") {
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

// copyDir recursively copies src to dst.
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

		tgtPath := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(tgtPath, 0o755)
		}

		return copyFile(path, tgtPath)
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
