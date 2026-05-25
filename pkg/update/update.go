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

// Package update re-installs an agentpack plugin from its stored source,
// comparing the old and new resolved SHAs to detect whether an update
// actually occurred.
package update

import (
	"context"
	"fmt"

	"github.com/retr0h/agentpack/pkg/install"
	"github.com/retr0h/agentpack/pkg/registry"
)

// Options configures an update run.
type Options struct {
	// Name is the plugin identifier to update.
	Name string
}

// Result holds the outcome of an update.
type Result struct {
	// Name is the plugin that was updated.
	Name string

	// OldSHA is the git commit SHA before the update.
	OldSHA string

	// NewSHA is the git commit SHA after the update.
	NewSHA string

	// Version is the new version string.
	Version string

	// Updated is true when OldSHA != NewSHA.
	Updated bool
}

// Run re-installs the named plugin from its stored source. It compares the
// old and new resolved SHAs to determine whether a new version was installed.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Load the existing registry manifest to get the source.
	m, err := registry.Load(opts.Name)
	if err != nil {
		return nil, fmt.Errorf("load registry manifest: %w", err)
	}

	oldSHA := m.SHA

	// Re-install from the stored source.
	installResult, err := install.Run(ctx, install.Options{
		Source: m.Source,
	})
	if err != nil {
		return nil, fmt.Errorf("re-install: %w", err)
	}

	return &Result{
		Name:    installResult.Name,
		OldSHA:  oldSHA,
		NewSHA:  installResult.SHA,
		Version: installResult.Version,
		Updated: oldSHA != installResult.SHA,
	}, nil
}
