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
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/retr0h/gilt/v2/pkg/config"
	"github.com/retr0h/gilt/v2/pkg/repositories"
)

// GitFetcher clones a git repository into a destination path using gilt
// as the underlying clone engine.
type GitFetcher struct{}

// Fetch clones the repository identified by source into dest. The source
// format is:
//
//   - "github.com/org/repo" — resolved to https://github.com/org/repo.git
//   - "https://github.com/org/repo.git" — used as-is
//   - Either form may include "#ref" to specify a tag, branch, or SHA.
func (f *GitFetcher) Fetch(ctx context.Context, source string, dest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	rawURL, ref, err := parseGitSource(source)
	if err != nil {
		return err
	}

	gitURL := toGitURL(rawURL)

	if ref == "" {
		ref = "HEAD"
	}

	cacheDir, err := defaultCacheDir()
	if err != nil {
		return fmt.Errorf("cache dir: %w", err)
	}

	cfg := config.Repositories{
		GiltDir: cacheDir,
		Repositories: []config.Repository{{
			Git:     gitURL,
			Version: ref,
			DstDir:  dest,
		}},
	}

	gilt := repositories.New(cfg, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	})))

	if err := gilt.Overlay(); err != nil {
		return fmt.Errorf("git clone %s: %w", source, err)
	}

	return nil
}

func defaultCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".agentpack", "cache")
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
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}

	return true
}
