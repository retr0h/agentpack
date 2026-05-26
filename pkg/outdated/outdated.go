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
//
// Usage:
//
//	entries, err := outdated.Run(ctx, nil) // all installed plugins
//	entries, err := outdated.RunWithOptions(ctx, outdated.Options{
//	    Names: []string{"my-plugin"},
//	    OnStep: func(name string) { fmt.Println("checking", name) },
//	})
//
// The Registry and RemoteChecker fields in Options accept nil, in which case
// the production implementations are used.
package outdated

import (
	"context"
	"fmt"

	"github.com/retr0h/agentpack/pkg/fetcher"
	"github.com/retr0h/agentpack/pkg/registry"
)

// Registry lists all installed package manifests from the registry store.
// Implement this interface to inject a test double in place of registry.List.
type Registry interface {
	List() ([]*registry.PackageManifest, error)
}

// RemoteChecker queries a remote git repository's refs without cloning.
// Implement this interface to inject a test double in place of fetcher.LsRemote.
type RemoteChecker interface {
	LsRemote(ctx context.Context, source string) (map[string]string, error)
}

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

	// Registry overrides the registry backend. When nil the default
	// registry.List / registry.Load implementation is used.
	Registry Registry

	// RemoteChecker overrides the remote HEAD resolver. When nil the default
	// fetcher.LsRemote implementation is used.
	RemoteChecker RemoteChecker
}

// defaultRegistry wraps the package-level registry functions to satisfy Registry.
type defaultRegistry struct{}

func (defaultRegistry) List() ([]*registry.PackageManifest, error) {
	return registry.List()
}

// defaultRemoteChecker wraps fetcher.LsRemote to satisfy RemoteChecker.
type defaultRemoteChecker struct{}

func (defaultRemoteChecker) LsRemote(ctx context.Context, source string) (map[string]string, error) {
	return fetcher.LsRemote(ctx, source)
}

// Run checks all installed plugins (or the named ones from names) for
// newer versions by calling LsRemote on their stored source URLs.
func Run(ctx context.Context, names []string) ([]Entry, error) {
	return RunWithOptions(ctx, Options{Names: names})
}

// RunWithOptions is like Run but accepts an Options struct for richer control.
func RunWithOptions(ctx context.Context, opts Options) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	reg := opts.Registry
	if reg == nil {
		reg = defaultRegistry{}
	}

	checker := opts.RemoteChecker
	if checker == nil {
		checker = defaultRemoteChecker{}
	}

	var manifests []*registry.PackageManifest

	if len(opts.Names) == 0 {
		all, err := reg.List()
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

		remoteSHA, lsErr := resolveRemoteHead(ctx, checker, m.Source)
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

// resolveRemoteHead calls checker.LsRemote and returns the SHA of HEAD.
func resolveRemoteHead(ctx context.Context, checker RemoteChecker, source string) (string, error) {
	refs, err := checker.LsRemote(ctx, source)
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
