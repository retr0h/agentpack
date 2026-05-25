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
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"
	gogithttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/jdx/go-netrc"
)

// netrcAuth reads ~/.netrc and returns HTTP BasicAuth for the host extracted
// from gitURL. It returns nil when no credentials are found or on any error,
// so callers always get a usable (possibly unauthenticated) auth value.
func netrcAuth(gitURL string) transport.AuthMethod {
	host := extractHost(gitURL)
	if host == "" {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	netrcPath := filepath.Join(home, ".netrc")

	n, err := netrc.Parse(netrcPath)
	if err != nil {
		return nil
	}

	machine := n.Machine(host)
	if machine == nil {
		return nil
	}

	return &gogithttp.BasicAuth{
		Username: machine.Get("login"),
		Password: machine.Get("password"),
	}
}

// extractHost returns the hostname from a git URL in any of the common forms:
//
//   - https://github.com/org/repo.git → "github.com"
//   - http://github.com/org/repo.git  → "github.com"
//   - git@github.com:org/repo.git     → "github.com"
//   - github.com/org/repo             → "github.com"
func extractHost(gitURL string) string {
	// Strip scheme.
	stripped := gitURL
	for _, scheme := range []string{"https://", "http://"} {
		if strings.HasPrefix(gitURL, scheme) {
			stripped = gitURL[len(scheme):]
			break
		}
	}

	// Handle git@ SCP form: git@github.com:org/repo.git
	if strings.HasPrefix(stripped, "git@") {
		stripped = strings.TrimPrefix(stripped, "git@")
		if idx := strings.IndexByte(stripped, ':'); idx >= 0 {
			return stripped[:idx]
		}

		return stripped
	}

	// Everything before the first slash is the host.
	if idx := strings.IndexByte(stripped, '/'); idx >= 0 {
		return stripped[:idx]
	}

	return stripped
}
