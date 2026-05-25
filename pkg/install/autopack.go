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

package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/avfs/avfs/vfs/osfs"

	"github.com/retr0h/agentpack/pkg/archive"
	"github.com/retr0h/agentpack/pkg/metadata"
)

// contentDirs are the recognized content directories per ADR-001. Only these
// are packaged; everything else in the source repo is excluded.
var contentDirs = []string{"skills", "commands", "agents", "hooks", "mcp", "settings"}

// Swappable functions for testing.
var (
	// archivesDirFunc returns ~/.config/agentpack/archives. Swappable in tests.
	archivesDirFunc = defaultArchivesDir

	// archivesDirHome is the home-dir lookup used by defaultArchivesDir.
	// Swappable in tests to avoid writes to the real home directory.
	archivesDirHome = os.UserHomeDir
)

// defaultArchivesDir returns ~/.config/agentpack/archives, creating it if needed.
func defaultArchivesDir() (string, error) {
	home, err := archivesDirHome()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}

	dir := filepath.Join(home, ".config", "agentpack", "archives")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir archives dir: %w", err)
	}

	return dir, nil
}

// archivesDir calls archivesDirFunc, which is swappable for tests.
func archivesDir() (string, error) {
	return archivesDirFunc()
}

// autoPackage walks the recognized content dirs inside cloneDir and produces a
// .agentpack archive. It applies skill and agent filters when non-empty. The
// archive is written to a temp file whose path is returned. The caller owns the
// file and should clean it up after use.
//
// Archive layout:
//
//	skills/{name}/…   (or subset when skillFilter is set)
//	commands/…
//	agents/{name}/…   (or subset when agentFilter is set)
//	hooks/…
//	mcp/…
//	settings/…
//	.agentpack/metadata.json
//	.agentpack/checksums.txt
func autoPackage(
	ctx context.Context,
	cloneDir string,
	name string,
	sha string,
	skillFilter []string,
	agentFilter []string,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var files []archive.FileEntry

	for _, content := range contentDirs {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		contentPath := filepath.Join(cloneDir, content)
		if _, err := os.Stat(contentPath); os.IsNotExist(err) {
			continue
		}

		var walkRoots []string

		switch content {
		case "skills":
			walkRoots = filteredSubdirs(contentPath, skillFilter)
		case "agents":
			walkRoots = filteredSubdirs(contentPath, agentFilter)
		default:
			walkRoots = []string{contentPath}
		}

		for _, root := range walkRoots {
			if err := ctx.Err(); err != nil {
				return "", err
			}

			entries, err := walkContentDir(cloneDir, root)
			if err != nil {
				return "", fmt.Errorf("walk %s: %w", content, err)
			}

			files = append(files, entries...)
		}
	}

	// Build metadata — use the SHA from the git clone.
	meta := &metadata.Metadata{
		Name:           name,
		Version:        "latest",
		GitCommitSHA:   sha,
		BuildTimestamp: time.Now().UTC().Format(time.RFC3339),
		BuilderVersion: "dev",
		Platform:       runtime.GOOS + "-" + runtime.GOARCH,
	}

	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling metadata: %w", err)
	}

	files = append(files, archive.FileEntry{
		ArchivePath: ".agentpack/metadata.json",
		Content:     metaJSON,
	})

	// Compute checksums over content files (excluding the checksum file itself).
	checksumContent, err := computeChecksums(files)
	if err != nil {
		return "", err
	}

	files = append(files, archive.FileEntry{
		ArchivePath: ".agentpack/checksums.txt",
		Content:     checksumContent,
	})

	// Write the archive to a temp file.
	tmpFile, err := os.CreateTemp("", "agentpack-auto-*.agentpack")
	if err != nil {
		return "", fmt.Errorf("create temp archive: %w", err)
	}

	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()

	vfs := osfs.NewWithNoIdm()

	if err := archive.Create(ctx, vfs, tmpPath, files); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("create archive: %w", err)
	}

	return tmpPath, nil
}

// storeArchive copies the archive at srcPath to the archives directory and
// returns the stored path. The naming follows ADR-001:
// {name}@{sha}.agentpack for git sources.
func storeArchive(srcPath, name, sha string) (string, error) {
	dir, err := archivesDir()
	if err != nil {
		return "", err
	}

	baseName := fmt.Sprintf("%s@%s", name, shortSHA(sha))
	dstPath := filepath.Join(dir, baseName+".agentpack")

	if err := copyFileAtomic(srcPath, dstPath); err != nil {
		return "", fmt.Errorf("store archive: %w", err)
	}

	// Write the archive SHA256 alongside the package for tamper detection.
	data, err := os.ReadFile(dstPath)
	if err == nil {
		h := sha256.Sum256(data)
		shaPath := filepath.Join(dir, baseName+".sha256")
		_ = os.WriteFile(shaPath, []byte(hex.EncodeToString(h[:])+"\n"), 0o644)
	}

	return dstPath, nil
}

// copyFileAtomic copies src to dst by reading src and writing to a temp file
// in the same directory as dst, then renaming into place.
func copyFileAtomic(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() { _ = srcFile.Close() }()

	tmpFile, err := os.CreateTemp(filepath.Dir(dst), ".agentpack-store-*")
	if err != nil {
		return fmt.Errorf("create temp for store: %w", err)
	}

	tmpPath := tmpFile.Name()

	if _, err := io.Copy(tmpFile, srcFile); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("copy to store: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp: %w", err)
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename to store: %w", err)
	}

	return nil
}

// filteredSubdirs returns the absolute paths of first-level subdirectories
// inside contentDir that match filter. When filter is empty all subdirectories
// are returned, plus contentDir itself when no subdirectories exist (flat
// layout).
func filteredSubdirs(contentDir string, filter []string) []string {
	entries, err := os.ReadDir(contentDir)
	if err != nil {
		// If we cannot read the dir, return the dir itself and let the walk
		// surface the error.
		return []string{contentDir}
	}

	// Check whether the content dir contains only files (flat layout, e.g.
	// commands/*.md). If so treat the whole dir as one root.
	hasSubdir := false

	for _, e := range entries {
		if e.IsDir() {
			hasSubdir = true
			break
		}
	}

	if !hasSubdir {
		return []string{contentDir}
	}

	// Subdirectory layout (e.g. skills/{name}/SKILL.md).
	if len(filter) == 0 {
		// Include all subdirs.
		var roots []string
		for _, e := range entries {
			if e.IsDir() {
				roots = append(roots, filepath.Join(contentDir, e.Name()))
			}
		}
		return roots
	}

	// Build a set for O(1) lookup.
	want := make(map[string]struct{}, len(filter))
	for _, f := range filter {
		want[f] = struct{}{}
	}

	var roots []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, ok := want[e.Name()]; ok {
			roots = append(roots, filepath.Join(contentDir, e.Name()))
		}
	}

	return roots
}

// walkContentDir recursively collects FileEntry items for every file under
// root. The ArchivePath for each file is the path relative to cloneDir so that
// the archive preserves the content dir prefix (e.g. skills/review/SKILL.md).
func walkContentDir(cloneDir, root string) ([]archive.FileEntry, error) {
	var entries []archive.FileEntry

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(cloneDir, path)
		if err != nil {
			return fmt.Errorf("rel path %s: %w", path, err)
		}

		entries = append(entries, archive.FileEntry{
			Src:         path,
			ArchivePath: rel,
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	return entries, nil
}

// computeChecksums produces the .agentpack/checksums.txt content (sha256sum
// format) for the given file entries. Virtual files (Content != nil) are hashed
// in memory; on-disk files (Src != "") are hashed by reading from disk.
func computeChecksums(files []archive.FileEntry) ([]byte, error) {
	var buf []byte

	for _, f := range files {
		var hash string

		if f.Src != "" {
			data, err := os.ReadFile(f.Src)
			if err != nil {
				return nil, fmt.Errorf("read %s for checksum: %w", f.Src, err)
			}

			h := sha256.Sum256(data)
			hash = hex.EncodeToString(h[:])
		} else {
			h := sha256.Sum256(f.Content)
			hash = hex.EncodeToString(h[:])
		}

		buf = fmt.Appendf(buf, "%s  %s\n", hash, f.ArchivePath)
	}

	return buf, nil
}
