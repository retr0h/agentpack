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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/retr0h/agentpack/pkg/archive"
	"github.com/retr0h/agentpack/pkg/checksum"
	"github.com/retr0h/agentpack/pkg/fetcher"
	"github.com/retr0h/agentpack/pkg/metadata"
	"github.com/retr0h/agentpack/pkg/registry"
	"github.com/retr0h/agentpack/pkg/target"
)

// Swappable OS functions for testing.
var (
	// osCreateTemp is a swappable wrapper so tests can inject temp-file
	// creation failures.
	osCreateTemp = os.CreateTemp
	osMkdirTemp  = os.MkdirTemp

	// registrySave is a swappable wrapper around registry.Save so tests can
	// prevent writes to the real ~/.config/agentpack/packages/ directory.
	registrySave = registry.Save
)

// Options configures an install run.
type Options struct {
	// Source is the local path or URL to the .agentpack archive.
	Source string

	// Dir is the root directory for installation (cwd for local, home for global).
	Dir string

	// Skills restricts the install to named skills only. Each value is matched
	// against the skill subdirectory name (e.g. "review" matches
	// skills/review/SKILL.md). When empty all skills are installed.
	Skills []string

	// Agents restricts the install to named agents only. When empty all agents
	// are installed.
	Agents []string

	// OriginalSource preserves the user-facing source URL when Source is
	// overwritten to point at a local archive during the build-first pipeline.
	OriginalSource string

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
	// FileCounts maps target display-name → number of files installed.
	FileCounts map[string]int
}

// Run installs from any source: .agentpack archive, git repo, or local path.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	f, err := fetcher.New(opts.Source)
	if err != nil {
		return nil, fmt.Errorf("fetcher: %w", err)
	}

	// Git source: clone → install directly from the repo contents.
	if _, isGit := f.(*fetcher.GitFetcher); isGit {
		return runFromGit(ctx, opts, f)
	}

	return runFromArchive(ctx, opts, f)
}

func runFromGit(ctx context.Context, opts Options, f fetcher.Fetcher) (*Result, error) {
	cloneDir, err := osMkdirTemp("", "agentpack-git-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(cloneDir) }()

	// Use FetchWithResult so we capture the resolved commit SHA.
	gf, _ := f.(*fetcher.GitFetcher)

	sha, err := gf.FetchWithResult(ctx, opts.Source, cloneDir)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	name := nameFromSource(opts.Source)

	// Build a .agentpack archive from the cloned repo contents. This ensures
	// every install path produces a verifiable, content-filtered archive per
	// ADR-001 regardless of whether the source repo ships an agentpack.yaml.
	archivePath, err := autoPackage(ctx, cloneDir, name, sha, opts.Skills, opts.Agents)
	if err != nil {
		return nil, fmt.Errorf("auto-package: %w", err)
	}
	defer func() { _ = os.Remove(archivePath) }()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Persist the archive to the archives store so subsequent reinstalls do not
	// require a network fetch.
	storedPath, err := storeArchive(archivePath, name, sha)
	if err != nil {
		// Non-fatal: we still have the temp archive, proceed with install.
		storedPath = archivePath
	}

	// Install using the standard archive path so checksum verification and
	// metadata parsing are applied uniformly.
	archiveOpts := opts
	archiveOpts.OriginalSource = opts.Source
	archiveOpts.Source = storedPath

	archiveFetcher, err := fetcher.New(storedPath)
	if err != nil {
		return nil, fmt.Errorf("fetcher for stored archive: %w", err)
	}

	return runFromArchive(ctx, archiveOpts, archiveFetcher)
}

func runFromArchive(ctx context.Context, opts Options, f fetcher.Fetcher) (*Result, error) {
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

	meta, err := findAndReadMetadata(tmpDir)
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return installFromDir(ctx, opts, tmpDir, meta)
}

// nameFromSource extracts a plugin name from a source URL.
func nameFromSource(source string) string {
	s := source
	if idx := strings.LastIndex(s, "#"); idx >= 0 {
		s = s[:idx]
	}

	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimSuffix(s, "/")

	if idx := strings.LastIndex(s, "/"); idx >= 0 {
		return s[idx+1:]
	}

	return s
}

// installFromDir is the shared install path for both archive and git sources.
func installFromDir(ctx context.Context, opts Options, sourceDir string, meta *metadata.Metadata) (*Result, error) {
	targets := opts.Targets
	if len(targets) == 0 {
		targets = target.Detected()
	}

	dir := opts.Dir
	if dir == "" {
		dir, _ = os.Getwd()
	}

	installOpts := target.InstallOpts{
		Name:    meta.Name,
		Version: meta.Version,
		Meta:    meta,
		Dir:     dir,
	}

	dirs := make(map[string]string)
	fileCounts := make(map[string]int)

	var allFiles []registry.InstalledFile

	for _, tgt := range targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		srcDir, err := copyToTemp(ctx, sourceDir)
		if err != nil {
			return nil, fmt.Errorf("prepare source for %s: %w", tgt.Name(), err)
		}

		installOpts.SourceDir = srcDir

		if installErr := tgt.Install(ctx, installOpts); installErr != nil {
			_ = os.RemoveAll(srcDir)

			return nil, fmt.Errorf("install to %s: %w", tgt.Name(), installErr)
		}

		// Collect installed files by scanning the target's install dirs.
		installed, collectErr := collectTargetFiles(dir, tgt, srcDir)
		if collectErr != nil {
			_ = os.RemoveAll(srcDir)

			return nil, fmt.Errorf("collect files for %s: %w", tgt.Name(), collectErr)
		}

		allFiles = append(allFiles, installed...)
		fileCounts[tgt.DisplayName()] = len(installed)

		_ = os.RemoveAll(srcDir)

		dirs[tgt.DisplayName()] = tgt.Name()
	}

	// Save registry manifest.
	manifest := &registry.PackageManifest{
		Name:      meta.Name,
		Source:    registrySource(opts),
		SHA:       meta.GitCommitSHA,
		Version:   meta.Version,
		Installed: time.Now().UTC().Format(time.RFC3339),
		Files:     allFiles,
	}
	if saveErr := registrySave(manifest); saveErr != nil {
		return nil, fmt.Errorf("save registry manifest: %w", saveErr)
	}

	return &Result{
		Name:       meta.Name,
		Version:    meta.Version,
		SHA:        shortSHA(meta.GitCommitSHA),
		Dirs:       dirs,
		FileCounts: fileCounts,
	}, nil
}

// collectInstalledFiles walks dir and returns an InstalledFile record for
// every regular file it contains. The SHA256 is computed from the file content.
// collectTargetFiles scans only the content dirs that exist in the source
// (skills/, commands/, agents/) and records what was copied to the install dir.
func collectTargetFiles(installDir string, tgt target.Target, srcDir string) ([]registry.InstalledFile, error) {
	targetContentDirs := []string{"skills", "commands", "agents"}
	var allFiles []registry.InstalledFile

	for _, content := range targetContentDirs {
		srcContent := filepath.Join(srcDir, content)
		if _, err := os.Stat(srcContent); os.IsNotExist(err) {
			continue
		}

		// Walk the source content dir and record relative paths.
		err := filepath.WalkDir(srcContent, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return walkErr
			}

			rel, _ := filepath.Rel(srcDir, path)
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}

			h := sha256.Sum256(data)
			allFiles = append(allFiles, registry.InstalledFile{
				Path:   rel,
				SHA256: hex.EncodeToString(h[:]),
				Target: tgt.Name(),
				Dir:    installDir,
			})

			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return allFiles, nil
}

func collectInstalledFiles(dir, targetName string) ([]registry.InstalledFile, error) {
	var files []registry.InstalledFile

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return fmt.Errorf("rel path: %w", relErr)
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}

		h := sha256.Sum256(data)

		files = append(files, registry.InstalledFile{
			Path:   rel,
			SHA256: hex.EncodeToString(h[:]),
			Target: targetName,
			Dir:    dir,
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	return files, nil
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

func registrySource(opts Options) string {
	if opts.OriginalSource != "" {
		return opts.OriginalSource
	}
	return opts.Source
}
