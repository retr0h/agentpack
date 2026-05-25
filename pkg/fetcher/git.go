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
	"os"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	gogitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

// GitFetcher clones a git repository into a destination path using go-git
// as the underlying clone engine.
type GitFetcher struct{}

// FetchWithResult clones the repository identified by source into dest and
// returns the resolved SHA of the checked-out commit. The source format is:
//
//   - "github.com/org/repo"            — resolved to https://github.com/org/repo.git
//   - "https://github.com/org/repo.git" — used as-is
//   - Either form may include "#ref" to specify a tag, branch, or SHA.
func (f *GitFetcher) FetchWithResult(ctx context.Context, source string, dest string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	rawURL, ref, err := parseGitSource(source)
	if err != nil {
		return "", err
	}

	gitURL := toGitURL(rawURL)
	auth := netrcAuth(gitURL)

	cloneOpts := &gogit.CloneOptions{
		URL:  gitURL,
		Auth: auth,
	}

	// When ref is a branch or empty, clone with that branch directly.
	// When ref looks like a SHA, clone the default branch then checkout.
	// When ref looks like a tag, set the ReferenceName to the tag.
	var resolveAfterClone bool

	switch {
	case ref == "" || ref == "HEAD":
		// no ref — clone HEAD of default branch
	case isSHA(ref):
		// SHA checkout: clone default then checkout commit
		resolveAfterClone = true
	case isTagRef(ref):
		cloneOpts.ReferenceName = plumbing.NewTagReferenceName(ref)
	default:
		// treat as branch
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(ref)
	}

	cloneOpts.NoCheckout = resolveAfterClone

	cacheDir, err := defaultCacheDir()
	if err != nil {
		return "", fmt.Errorf("cache dir: %w", err)
	}

	// Use cache dir per remote to avoid redundant full clones when possible.
	cacheKey := sanitizePath(gitURL)
	cachedRepo := filepath.Join(cacheDir, cacheKey)

	repo, err := gogit.PlainCloneContext(ctx, cachedRepo, false, cloneOpts)
	if err != nil {
		if err == gogit.ErrRepositoryAlreadyExists {
			repo, err = gogit.PlainOpen(cachedRepo)
			if err != nil {
				return "", fmt.Errorf("open cached repo: %w", err)
			}

			// Fetch latest from remote.
			if fetchErr := repo.FetchContext(ctx, &gogit.FetchOptions{
				Auth:  auth,
				Force: true,
			}); fetchErr != nil && fetchErr != gogit.NoErrAlreadyUpToDate {
				return "", fmt.Errorf("fetch: %w", fetchErr)
			}
		} else {
			return "", fmt.Errorf("git clone %s: %w", source, err)
		}
	}

	// If SHA checkout is required, do it now.
	if resolveAfterClone && ref != "" {
		w, werr := repo.Worktree()
		if werr != nil {
			return "", fmt.Errorf("worktree: %w", werr)
		}

		hash := plumbing.NewHash(ref)
		if checkErr := w.Checkout(&gogit.CheckoutOptions{Hash: hash}); checkErr != nil {
			return "", fmt.Errorf("checkout %s: %w", ref, checkErr)
		}
	}

	// Copy cached worktree into dest (excluding .git/).
	if copyErr := copyWorktree(ctx, cachedRepo, dest); copyErr != nil {
		return "", fmt.Errorf("copy worktree: %w", copyErr)
	}

	// Resolve current HEAD SHA.
	head, headErr := repo.Head()
	if headErr != nil {
		return "", fmt.Errorf("resolve HEAD: %w", headErr)
	}

	return head.Hash().String(), nil
}

// Fetch implements the Fetcher interface. It clones the repository into dest.
// Use FetchWithResult when the resolved SHA is needed.
func (f *GitFetcher) Fetch(ctx context.Context, source string, dest string) error {
	_, err := f.FetchWithResult(ctx, source, dest)
	return err
}

// LsRemote lists all references from a remote repository without cloning it.
// It returns a map of refname → SHA.
func LsRemote(ctx context.Context, rawURL string) (map[string]string, error) {
	gitURL := toGitURL(rawURL)
	auth := netrcAuth(gitURL)

	remote := gogit.NewRemote(nil, &gogitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{gitURL},
	})

	refs, err := remote.ListContext(ctx, &gogit.ListOptions{Auth: auth})
	if err != nil {
		return nil, fmt.Errorf("ls-remote %s: %w", gitURL, err)
	}

	result := make(map[string]string, len(refs))
	for _, ref := range refs {
		result[ref.Name().String()] = ref.Hash().String()
	}

	return result, nil
}

// defaultCacheDir returns ~/.config/agentpack/cache, creating it if needed.
func defaultCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(home, ".config", "agentpack", "cache")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}

	return dir, nil
}

// parseGitSource splits "host/org/repo#ref" into (rawURL, ref).
func parseGitSource(source string) (rawURL string, ref string, err error) {
	if source == "" {
		return "", "", fmt.Errorf("git source must not be empty")
	}

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
func toGitURL(rawURL string) string {
	if strings.HasPrefix(rawURL, "https://") ||
		strings.HasPrefix(rawURL, "http://") ||
		strings.HasPrefix(rawURL, "/") ||
		strings.HasPrefix(rawURL, "./") ||
		strings.HasPrefix(rawURL, "../") {
		return rawURL
	}

	url := "https://" + rawURL
	if !strings.HasSuffix(url, ".git") {
		url += ".git"
	}

	return url
}

// isSHA returns true when ref looks like a git SHA (hex characters only).
func isSHA(ref string) bool {
	if len(ref) < 4 || len(ref) > 40 {
		return false
	}

	for _, c := range ref {
		isDigit := c >= '0' && c <= '9'
		isLower := c >= 'a' && c <= 'f'
		isUpper := c >= 'A' && c <= 'F'

		if !isDigit && !isLower && !isUpper {
			return false
		}
	}

	return true
}

// isTagRef heuristically detects tag-like refs (e.g. "v1.0.0").
// Branches and tags are indistinguishable syntactically, so we use the
// presence of a leading 'v' followed by a digit as a tag signal.
func isTagRef(ref string) bool {
	return len(ref) > 1 && ref[0] == 'v' && ref[1] >= '0' && ref[1] <= '9'
}

// sanitizePath converts a URL into a filesystem-safe directory name.
func sanitizePath(url string) string {
	r := strings.NewReplacer(
		"https://", "",
		"http://", "",
		"/", "_",
		":", "_",
		".git", "",
	)

	return r.Replace(url)
}

// copyWorktree copies all files from src to dst, skipping the .git/ directory.
func copyWorktree(ctx context.Context, src string, dst string) error {
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

		// Skip the .git directory.
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}

			return nil
		}

		if rel == "." {
			return nil
		}

		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		return copyFileGit(path, target)
	})
}

// copyFileGit copies a single file preserving its permission bits.
func copyFileGit(src string, dst string) error {
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
