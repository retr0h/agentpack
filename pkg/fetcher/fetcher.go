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

// New returns the appropriate Fetcher for the given source URI.
// It inspects the source string to select the matching backend:
//   - local path (absolute, relative, or home-relative) → FileFetcher
//   - http:// or https:// → HTTPFetcher
//   - s3:// → error (not yet implemented)
//   - gs:// → error (not yet implemented)
//   - unknown scheme → error
func New(source string) (Fetcher, error) {
	switch {
	case strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://"):
		return &HTTPFetcher{}, nil
	case strings.HasPrefix(source, "s3://"):
		return nil, fmt.Errorf("s3 backend not yet implemented")
	case strings.HasPrefix(source, "gs://"):
		return nil, fmt.Errorf("gs backend not yet implemented")
	case strings.Contains(source, "://"):
		return nil, fmt.Errorf("unknown scheme in source: %q", source)
	default:
		// local path: absolute (/), relative (./), home-relative (~/), or bare name
		return &FileFetcher{}, nil
	}
}
