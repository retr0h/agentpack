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

// Package fetcher provides a backend interface and drivers for retrieving
// .agentpack archives from local paths or remote URLs.
//
// Usage:
//
//	f, err := fetcher.New("github.com/org/skills-repo")
//	if err != nil { ... }
//	err = f.Fetch(ctx, source, "/tmp/dest.agentpack")
//
//	// List remote refs without cloning.
//	refs, err := fetcher.LsRemote(ctx, "github.com/org/skills-repo")
//
// New selects the appropriate driver based on the source URI:
//   - github.com/, gitlab.com/, bitbucket.org/ (bare or https://) → GitFetcher
//   - ends with .git → GitFetcher
//   - https:// on a non-git host → HTTPFetcher
//   - local path (absolute, relative, or ~/…) → FileFetcher
package fetcher

import (
	"context"
	"fmt"
	"strings"
)

// Fetcher retrieves a .agentpack archive from a source URI to a local path.
type Fetcher interface {
	Fetch(ctx context.Context, source string, dest string) error
}

// gitHosts lists the known hosting prefixes that trigger GitFetcher selection
// when no explicit scheme is present.
var gitHosts = []string{
	"github.com/",
	"gitlab.com/",
	"bitbucket.org/",
}

// New returns the appropriate Fetcher for the given source URI.
// It inspects the source string to select the matching backend:
//   - github.com/, gitlab.com/, bitbucket.org/ — bare or https:// → GitFetcher
//   - ends with .git → GitFetcher
//   - https://example.com/archive.agentpack (non-git host) → HTTPFetcher
//   - http:// (non-git host) → HTTPFetcher
//   - s3:// → error (not yet implemented)
//   - gs:// → error (not yet implemented)
//   - unknown scheme → error
//   - local path (absolute, relative, or home-relative) → FileFetcher
func New(source string) (Fetcher, error) {
	source = ExpandShorthand(source)

	// Strip #ref (legacy), :selectors (ADR-010), and @ref (ADR-010) for
	// scheme detection. Handle "://" schemes carefully.
	bare := source
	if idx := strings.LastIndex(bare, "#"); idx >= 0 {
		bare = bare[:idx]
	}

	// Strip :selectors only when the colon is NOT part of a "://" scheme.
	if idx := strings.Index(bare, ":"); idx >= 0 {
		isScheme := idx+2 < len(bare) && bare[idx+1] == '/' && bare[idx+2] == '/'
		if !isScheme {
			bare = bare[:idx]
		}
	}

	// Strip @ref — but not from URLs that contain "://" (to preserve host).
	if !strings.Contains(bare, "://") {
		if idx := strings.Index(bare, "@"); idx >= 0 {
			bare = bare[:idx]
		}
	}

	switch {
	case isGitSource(bare):
		return &GitFetcher{}, nil
	case strings.HasPrefix(source, "s3://"):
		return nil, fmt.Errorf("s3 backend not yet implemented")
	case strings.HasPrefix(source, "gs://"):
		return nil, fmt.Errorf("gs backend not yet implemented")
	case strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://"):
		return &HTTPFetcher{}, nil
	case strings.Contains(source, "://"):
		return nil, fmt.Errorf("unknown scheme in source: %q", source)
	default:
		// local path: absolute (/), relative (./), home-relative (~/), or bare name
		return &FileFetcher{}, nil
	}
}

// ExpandShorthand converts owner/repo shorthand to github.com/owner/repo.
// A source is shorthand when it has exactly one slash, no dots, no scheme,
// and no path separators that suggest a local path.
//
// The source may include @ref and :selector suffixes per ADR-010; these are
// stripped before checking the shorthand pattern and preserved in the output.
func ExpandShorthand(source string) string {
	bare := source

	// Strip #ref fragment (legacy) for pattern matching.
	if idx := strings.LastIndex(bare, "#"); idx >= 0 {
		bare = bare[:idx]
	}

	// Strip :selectors (ADR-010) for pattern matching.
	if idx := strings.Index(bare, ":"); idx >= 0 {
		bare = bare[:idx]
	}

	// Strip @ref (ADR-010) for pattern matching.
	if idx := strings.Index(bare, "@"); idx >= 0 {
		bare = bare[:idx]
	}

	if strings.Contains(bare, "://") ||
		strings.Contains(bare, ".") ||
		strings.HasPrefix(bare, "/") ||
		strings.HasPrefix(bare, "~") ||
		strings.HasPrefix(bare, ".") ||
		strings.Count(bare, "/") != 1 {
		return source
	}

	return "github.com/" + source
}

// isGitSource returns true when bare (URL without fragment) looks like a git
// repository reference: it matches a known host prefix (with or without an
// https:// scheme) or ends with ".git".
func isGitSource(bare string) bool {
	// Strip any https:// or http:// prefix for host comparison.
	stripped := bare
	for _, scheme := range []string{"https://", "http://"} {
		if strings.HasPrefix(bare, scheme) {
			stripped = bare[len(scheme):]
			break
		}
	}

	for _, host := range gitHosts {
		if strings.HasPrefix(stripped, host) {
			return true
		}
	}

	return strings.HasSuffix(bare, ".git")
}
