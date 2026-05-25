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

// Package outdated compares the installed SHA of each plugin against the
// current HEAD of its remote source using LsRemote (no clone required).
package outdated

import (
	"context"
	"fmt"

	"github.com/retr0h/agentpack/pkg/fetcher"
	"github.com/retr0h/agentpack/pkg/registry"
)

// Entry describes the outdated status of a single plugin.
type Entry struct {
	// Name is the plugin identifier.
	Name string

	// InstalledSHA is the commit SHA that is currently installed.
	InstalledSHA string

	// RemoteSHA is the HEAD commit SHA on the remote.
	RemoteSHA string

	// Outdated is true when InstalledSHA != RemoteSHA.
	Outdated bool
}

// Options configures an outdated check run.
type Options struct {
	// Names is the list of plugin names to check. When empty all installed
	// plugins are checked.
	Names []string

	// OnStep is called in real-time before each plugin's remote is queried.
	// When nil, no progress is reported.
	OnStep func(name string)
}

// Run checks all installed plugins (or the named ones from opts.Names) for
// newer versions by calling LsRemote on their stored source URLs.
func Run(ctx context.Context, names []string) ([]Entry, error) {
	return RunWithOptions(ctx, Options{Names: names})
}

// RunWithOptions is like Run but accepts an Options struct for richer control.
func RunWithOptions(ctx context.Context, opts Options) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var manifests []*registry.PackageManifest

	if len(opts.Names) == 0 {
		all, err := registry.List()
		if err != nil {
			return nil, fmt.Errorf("list registry: %w", err)
		}

		manifests = all
	} else {
		for _, n := range opts.Names {
			m, err := registry.Load(n)
			if err != nil {
				return nil, fmt.Errorf("load %s: %w", n, err)
			}

			manifests = append(manifests, m)
		}
	}

	entries := make([]Entry, 0, len(manifests))

	for _, m := range manifests {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if opts.OnStep != nil {
			opts.OnStep(m.Name)
		}

		remoteSHA, lsErr := resolveRemoteHead(ctx, m.Source)
		if lsErr != nil {
			// Non-fatal: include entry with empty remote SHA.
			entries = append(entries, Entry{
				Name:         m.Name,
				InstalledSHA: m.SHA,
				RemoteSHA:    "",
				Outdated:     false,
			})

			continue
		}

		entries = append(entries, Entry{
			Name:         m.Name,
			InstalledSHA: m.SHA,
			RemoteSHA:    remoteSHA,
			Outdated:     m.SHA != remoteSHA && remoteSHA != "",
		})
	}

	return entries, nil
}

// resolveRemoteHead calls LsRemote and returns the SHA of HEAD.
func resolveRemoteHead(ctx context.Context, source string) (string, error) {
	refs, err := fetcher.LsRemote(ctx, source)
	if err != nil {
		return "", err
	}

	// Try HEAD directly, then refs/heads/main, then refs/heads/master.
	for _, name := range []string{"HEAD", "refs/heads/main", "refs/heads/master"} {
		if sha, ok := refs[name]; ok && sha != "" {
			return sha, nil
		}
	}

	return "", fmt.Errorf("HEAD not found in remote refs for %s", source)
}
