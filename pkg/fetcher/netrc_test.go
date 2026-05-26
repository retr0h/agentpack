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

package fetcher_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/pkg/fetcher"
)

func TestExtractHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		gitURL   string
		wantHost string
	}{
		{
			name:     "https URL",
			gitURL:   "https://github.com/org/repo.git",
			wantHost: "github.com",
		},
		{
			name:     "http URL",
			gitURL:   "http://internal.example.com/repo.git",
			wantHost: "internal.example.com",
		},
		{
			name:     "git SCP form",
			gitURL:   "git@github.com:org/repo.git",
			wantHost: "github.com",
		},
		{
			name:     "git SCP form with no colon returns entire remainder",
			gitURL:   "git@github.com",
			wantHost: "github.com",
		},
		{
			name:     "bare host path",
			gitURL:   "github.com/org/repo",
			wantHost: "github.com",
		},
		{
			name:     "bare host no path",
			gitURL:   "example.com",
			wantHost: "example.com",
		},
		{
			name:     "empty string",
			gitURL:   "",
			wantHost: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := fetcher.ExtractHost(tt.gitURL)
			assert.Equal(t, tt.wantHost, got)
		})
	}
}

func TestNetrcAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		gitURL   string
		wantAuth bool // whether we expect non-nil auth
	}{
		{
			name:     "nonexistent host returns nil",
			gitURL:   "https://no-such-host.invalid/org/repo.git",
			wantAuth: false,
		},
		{
			name:     "empty URL returns nil",
			gitURL:   "",
			wantAuth: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			auth := fetcher.NetrcAuth(tt.gitURL)
			assert.Equal(t, tt.wantAuth, auth != nil)
		})
	}
}

// TestNetrcAuthWithNetrcFile exercises the machine-lookup and BasicAuth-return
// paths inside netrcAuth by creating a real .netrc in a temp directory and
// redirecting $HOME so os.UserHomeDir() finds it. These subtests cannot run
// in parallel because they mutate $HOME via t.Setenv.
func TestNetrcAuthWithNetrcFile(t *testing.T) {
	tests := []struct {
		name     string
		netrc    string
		gitURL   string
		wantAuth bool
	}{
		{
			name: "matching machine returns BasicAuth",
			netrc: "machine git.example.com\n" +
				"login myuser\n" +
				"password mypass\n",
			gitURL:   "https://git.example.com/org/repo.git",
			wantAuth: true,
		},
		{
			name:     "no matching machine returns nil",
			netrc:    "machine other.example.com\nlogin u\npassword p\n",
			gitURL:   "https://git.example.com/org/repo.git",
			wantAuth: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)

			netrcPath := filepath.Join(home, ".netrc")
			require.NoError(t, os.WriteFile(netrcPath, []byte(tt.netrc), 0o600))

			auth := fetcher.NetrcAuth(tt.gitURL)
			assert.Equal(t, tt.wantAuth, auth != nil)
		})
	}
}
