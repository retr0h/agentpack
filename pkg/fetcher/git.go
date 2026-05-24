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

package fetcher

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitCommand is swappable for testing.
var gitCommand = func(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "git", args...)
}

// osMkdirTemp is swappable for testing.
var osMkdirTemp = os.MkdirTemp

// osRemoveAll is swappable for testing.
var osRemoveAll = os.RemoveAll

// GitFetcher clones a git repository and copies its contents (minus .git/)
// to a destination path. It supports shallow clones for efficiency.
type GitFetcher struct{}

// CloneInto clones the repository identified by source directly into dest,
// preserving the .git directory. This is used by the sync pipeline so that
// build.Run can capture git metadata from the cloned working tree.
//
// The source format is the same as Fetch: bare host paths, https:// URLs,
// and optional "#ref" fragments are all supported.
func (f *GitFetcher) CloneInto(ctx context.Context, source string, dest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	rawURL, ref, err := parseGitSource(source)
	if err != nil {
		return err
	}

	gitURL := toGitURL(rawURL)

	return cloneRepo(ctx, gitURL, ref, dest)
}

// Fetch clones the repository identified by source into dest. The source
// format is:
//
//   - "github.com/org/repo" — resolved to https://github.com/org/repo.git
//   - "https://github.com/org/repo.git" — used as-is
//   - Either form may include "#ref" to specify a tag, branch, or SHA.
//
// When ref is a full or short SHA (hex characters only) the implementation
// clones the default branch then runs "git checkout <ref>". Otherwise it
// passes ref to "--branch" for a shallow clone.
func (f *GitFetcher) Fetch(ctx context.Context, source string, dest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	rawURL, ref, err := parseGitSource(source)
	if err != nil {
		return err
	}

	gitURL := toGitURL(rawURL)

	tmpDir, err := osMkdirTemp("", "agentpack-git-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	defer func() { _ = osRemoveAll(tmpDir) }()

	if err := cloneRepo(ctx, gitURL, ref, tmpDir); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := copyDir(ctx, tmpDir, dest); err != nil {
		return fmt.Errorf("copy repo contents: %w", err)
	}

	return nil
}

// parseGitSource splits "host/org/repo#ref" into (rawURL, ref). ref is empty
// when no fragment is present. rawURL still contains its scheme if one was
// provided.
func parseGitSource(source string) (rawURL string, ref string, err error) {
	if source == "" {
		return "", "", fmt.Errorf("git source must not be empty")
	}

	// Fragment separator.
	if idx := strings.LastIndex(source, "#"); idx >= 0 {
		rawURL = source[:idx]
		ref = source[idx+1:]
	} else {
		rawURL = source
	}

	if rawURL == "" {
		return "", "", fmt.Errorf("git source has empty URL before '#'")
	}

	return rawURL, ref, nil
}

// toGitURL converts a bare host/org/repo path into an https:// clone URL.
// Full https:// or http:// URLs, and absolute paths, are returned unchanged.
// A bare host path (e.g. "github.com/org/repo") receives an https:// prefix
// and a .git suffix.
func toGitURL(rawURL string) string {
	if strings.HasPrefix(rawURL, "https://") ||
		strings.HasPrefix(rawURL, "http://") ||
		strings.HasPrefix(rawURL, "/") ||
		strings.HasPrefix(rawURL, "./") ||
		strings.HasPrefix(rawURL, "../") {
		return rawURL
	}

	// Bare form: "github.com/org/repo" — add scheme and .git suffix.
	url := "https://" + rawURL
	if !strings.HasSuffix(url, ".git") {
		url += ".git"
	}

	return url
}

// isSHA returns true when ref looks like a git SHA (hex characters only).
// Both full 40-char and short 7-char forms are accepted.
func isSHA(ref string) bool {
	if len(ref) < 4 || len(ref) > 40 {
		return false
	}

	for _, c := range ref {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}

	return true
}

// cloneRepo performs a shallow git clone into destDir. destDir must already
// exist; git is invoked with destDir as the working directory and "." as the
// clone target so it populates the existing directory without creating a new
// one.
//
// When ref is a named tag or branch, --branch <ref> is passed for a shallow
// clone. When ref looks like a SHA, the default HEAD is cloned first and then
// checked out. When ref is empty, HEAD is cloned without a branch flag.
func cloneRepo(ctx context.Context, gitURL, ref, destDir string) error {
	if ref != "" && isSHA(ref) {
		// Clone HEAD first, then checkout the specific SHA.
		cmd := gitCommand(ctx, "clone", "--depth", "1", gitURL, ".")
		cmd.Dir = destDir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git clone: %w\n%s", err, out)
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		checkout := gitCommand(ctx, "checkout", ref)
		checkout.Dir = destDir
		if out, err := checkout.CombinedOutput(); err != nil {
			return fmt.Errorf("git checkout %s: %w\n%s", ref, err, out)
		}

		return nil
	}

	args := []string{"clone", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}

	args = append(args, gitURL, ".")
	cmd := gitCommand(ctx, args...)
	cmd.Dir = destDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone: %w\n%s", err, out)
	}

	return nil
}

// copyDir recursively copies all files from src to dest, skipping the .git/
// directory. dest is created if it does not exist.
func copyDir(ctx context.Context, src, dest string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}

		// Skip .git directory entirely.
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}

			return nil
		}

		target := filepath.Join(dest, rel)

		if d.IsDir() {
			info, statErr := d.Info()
			if statErr != nil {
				return statErr
			}

			return os.MkdirAll(target, info.Mode())
		}

		return copyFile(path, target)
	})
}

// copyFile copies a single file, preserving its permissions.
func copyFile(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}

	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}

	if err := os.WriteFile(dest, data, info.Mode()); err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}

	return nil
}
