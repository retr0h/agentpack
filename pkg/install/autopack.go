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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/avfs/avfs/vfs/osfs"
	"gopkg.in/yaml.v3"

	"github.com/retr0h/agentpack/internal/archive"
	"github.com/retr0h/agentpack/internal/checksum"
	"github.com/retr0h/agentpack/internal/gitutil"
	"github.com/retr0h/agentpack/internal/metadata"
	"github.com/retr0h/agentpack/pkg/safety"
)

// contentDirs are the recognized content directories per ADR-001. Only these
// are packaged; everything else in the source repo is excluded.
var contentDirs = []string{"skills", "commands", "agents", "hooks", "mcp", "settings"}

// dirToType maps each content directory name to its metadata entry type per
// ADR-009.
var dirToType = map[string]string{
	"skills":   "skill",
	"commands": "command",
	"hooks":    "hook",
	"agents":   "agent",
	"mcp":      "mcp",
	"settings": "config",
}

// defaultArchivesDir returns ~/.config/agentpack/archives, creating it if
// needed. It is the default for Installer.archivesDir; tests inject a
// replacement on the Installer instead of mutating a package global.
func defaultArchivesDir() (string, error) {
	return archivesDirForHome(os.UserHomeDir)
}

// archivesDirForHome resolves and creates the archives directory under the home
// returned by homeFn. Taking homeFn as a parameter (rather than a swappable
// global) keeps it a pure function tests can drive directly.
func archivesDirForHome(homeFn func() (string, error)) (string, error) {
	home, err := homeFn()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}

	dir := filepath.Join(home, ".config", "agentpack", "archives")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir archives dir: %w", err)
	}

	return dir, nil
}

func autoPackageWithVersion(
	ctx context.Context,
	cloneDir string,
	name string,
	sha string,
	version string,
	skillFilter []string,
	agentFilter []string,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var files []archive.FileEntry

	var entries []metadata.ContentEntry

	// A selective install (skill/ or agent/ selectors) packages only the
	// requested content types. When both kinds of selector are present we must
	// keep both dirs — restricting to "skills" alone silently drops the
	// requested agents.
	dirs := contentDirs
	if len(skillFilter) > 0 || len(agentFilter) > 0 {
		dirs = nil
		if len(skillFilter) > 0 {
			dirs = append(dirs, "skills")
		}
		if len(agentFilter) > 0 {
			dirs = append(dirs, "agents")
		}
	}

	for _, content := range dirs {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		contentPath := filepath.Join(cloneDir, content)
		if _, err := os.Stat(contentPath); errors.Is(err, os.ErrNotExist) {
			continue
		}

		var walkRoots []string

		switch content {
		case "skills", "agents":
			var filter []string
			if content == "skills" {
				filter = skillFilter
			} else {
				filter = agentFilter
			}

			walkRoots = filteredSubdirs(contentPath, filter)

			// Each subdirectory becomes an entry.
			for _, root := range walkRoots {
				entries = append(entries, metadata.ContentEntry{
					Name: filepath.Base(root),
					Type: dirToType[content],
				})
			}
		default:
			walkRoots = []string{contentPath}

			// The directory itself is one entry.
			entries = append(entries, metadata.ContentEntry{
				Name: content,
				Type: dirToType[content],
			})
		}

		for _, root := range walkRoots {
			if err := ctx.Err(); err != nil {
				return "", err
			}

			fileEntries, err := walkContentDir(cloneDir, root)
			if err != nil {
				return "", fmt.Errorf("walk %s: %w", content, err)
			}

			files = append(files, fileEntries...)
		}
	}

	// Classify content files for safety before embedding in metadata.
	contentMap, err := buildContentMap(files)
	if err != nil {
		return "", fmt.Errorf("read files for classification: %w", err)
	}

	classification, err := safety.Classify(contentMap)
	if err != nil {
		return "", fmt.Errorf("safety classification: %w", err)
	}

	// Build metadata — use the SHA from the git clone.
	meta := &metadata.Metadata{
		Name:           name,
		Version:        version,
		GitCommitSHA:   sha,
		BuildTimestamp: time.Now().UTC().Format(time.RFC3339),
		BuilderVersion: "dev",
		Platform:       runtime.GOOS + "-" + runtime.GOARCH,
		Content:        classification,
		Entries:        entries,
	}

	metaYAML, err := yaml.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("marshaling metadata: %w", err)
	}

	files = append(files, archive.FileEntry{
		ArchivePath: ".agentpack/metadata.yaml",
		Content:     metaYAML,
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
func (i *Installer) storeArchive(ctx context.Context, srcPath, name, sha string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	dir, err := i.archivesDir()
	if err != nil {
		return "", err
	}

	baseName := fmt.Sprintf("%s@%s", name, gitutil.ShortSHA(sha))
	dstPath := filepath.Join(dir, baseName+".agentpack")

	if err := copyFileAtomic(srcPath, dstPath); err != nil {
		return "", fmt.Errorf("store archive: %w", err)
	}

	// Write the archive SHA256 alongside the package for tamper detection.
	// A missing sidecar silently disables verification on reinstall, so a
	// failure to compute or write it is reported rather than swallowed. The
	// hash is streamed (not read whole) so a large archive cannot OOM.
	sum, err := checksum.ComputeFile(ctx, osfs.NewWithNoIdm(), dstPath)
	if err != nil {
		return "", fmt.Errorf("hash stored archive for sidecar: %w", err)
	}

	// The sidecar must sit at "<archive>.sha256" — the same convention the
	// verifier (verifyArchiveSidecar) and `agentpack verify` look up. Writing
	// "<base>.sha256" instead leaves it where nothing reads it.
	shaPath := dstPath + ".sha256"
	if err := os.WriteFile(shaPath, []byte(sum+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("write archive sidecar: %w", err)
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

// buildContentMap reads file content from disk (Src) or memory (Content) and
// returns a path->content map suitable for safety.Classify. Only content files
// (those without a .agentpack/ prefix) are included.
func buildContentMap(files []archive.FileEntry) (map[string][]byte, error) {
	m := make(map[string][]byte, len(files))

	for _, f := range files {
		if f.Src != "" {
			data, err := os.ReadFile(f.Src)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", f.Src, err)
			}

			m[f.ArchivePath] = data
		} else {
			m[f.ArchivePath] = f.Content
		}
	}

	return m, nil
}
