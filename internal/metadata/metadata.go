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

// Package metadata captures git SHA, version, and timestamp for archives.
package metadata

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/retr0h/agentpack/internal/safety"
)

// Metadata holds build and source-control information for a plugin archive.
type Metadata struct {
	Name           string                 `json:"name"`
	Version        string                 `json:"version"`
	GitCommitSHA   string                 `json:"gitCommitSHA"`
	GitBranch      string                 `json:"gitBranch"`
	BuildTimestamp string                 `json:"buildTimestamp"`
	BuilderVersion string                 `json:"builderVersion"`
	Platform       string                 `json:"platform"`
	Content        *safety.Classification `json:"content,omitempty"`
}

// Capture collects git state and build information from dir and returns a
// populated Metadata. name and version are passed through unchanged.
//
// Returns an error containing "not a git repository" when dir is not inside a
// git repository.
func Capture(ctx context.Context, dir string, name string, version string) (*Metadata, error) {
	sha, err := gitOutput(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git rev-parse HEAD: %w", err)
	}

	branch, err := gitOutput(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git rev-parse --abbrev-ref HEAD: %w", err)
	}

	return &Metadata{
		Name:           name,
		Version:        version,
		GitCommitSHA:   sha,
		GitBranch:      branch,
		BuildTimestamp: time.Now().UTC().Format(time.RFC3339),
		BuilderVersion: "dev",
		Platform:       runtime.GOOS + "-" + runtime.GOARCH,
	}, nil
}

// gitOutput runs git with args inside dir and returns trimmed stdout.
// It surfaces "not a git repository" from stderr when git exits non-zero.
// The context is used for cancellation via exec.CommandContext.
func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if strings.Contains(msg, "not a git repository") {
			return "", fmt.Errorf("not a git repository: %s", dir)
		}
		return "", fmt.Errorf("%s: %w", msg, err)
	}

	return strings.TrimSpace(stdout.String()), nil
}
