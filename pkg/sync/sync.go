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

// Package sync provides declarative plugin sync from a agentpack-packages.yaml.
package sync

import (
	"context"
	"fmt"
	"os"

	"github.com/avfs/avfs/vfs/osfs"
	"gopkg.in/yaml.v3"

	"github.com/retr0h/agentpack/pkg/build"
	"github.com/retr0h/agentpack/pkg/fetcher"
	"github.com/retr0h/agentpack/pkg/install"
)

// PackagesFile represents the top-level structure of agentpack-packages.yaml.
type PackagesFile struct {
	Packages []Package `yaml:"packages"`
}

// Package declares a single plugin source in agentpack-packages.yaml.
// Exactly one of Source or Git must be set.
//
//   - Source: a local path or URL to a pre-built .agentpack archive.
//   - Git: a git repository reference such as "github.com/org/repo". When
//     set, the sync pipeline clones the repo, builds any plugins defined in
//     agentpack.yaml found at the repo root, and installs the resulting
//     archives. Ref optionally pins a specific tag, branch, or SHA; when
//     omitted the default HEAD is used.
type Package struct {
	Name   string `yaml:"name"`
	Source string `yaml:"source"` // local path or URL to .agentpack archive
	Git    string `yaml:"git"`    // git repo (e.g. github.com/org/repo)
	Ref    string `yaml:"ref"`    // tag, branch, or SHA (default: HEAD)
}

// Result holds the outcome for one package in a sync run.
type Result struct {
	Name    string
	Version string
	Status  string // "installed", "up to date", "failed"
	Err     error
}

// Run reads configPath (a agentpack-packages.yaml) and installs or updates every
// declared package into all detected targets. Results are returned for every
// package, including failures. A non-nil error is returned only when the config
// file itself cannot be read or parsed. The pluginDir parameter is retained for
// API compatibility but is no longer used; target drivers determine install
// locations.
func Run(ctx context.Context, configPath string, _ string) ([]Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", configPath, err)
	}

	var pf PackagesFile
	if err := yaml.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("parse %s: %w", configPath, err)
	}

	results := make([]Result, 0, len(pf.Packages))

	for _, pkg := range pf.Packages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		var pkgResults []Result

		if pkg.Git != "" {
			pkgResults = syncGitPackage(ctx, pkg)
		} else {
			pkgResults = syncSourcePackage(ctx, pkg)
		}

		results = append(results, pkgResults...)
	}

	return results, nil
}

// syncSourcePackage installs a single package from a pre-built .agentpack
// archive pointed to by pkg.Source.
func syncSourcePackage(ctx context.Context, pkg Package) []Result {
	r, installErr := install.Run(ctx, install.Options{
		Source: pkg.Source,
	})

	if installErr != nil {
		return []Result{{
			Name:   pkg.Name,
			Status: "failed",
			Err:    installErr,
		}}
	}

	return []Result{{
		Name:    r.Name,
		Version: r.Version,
		Status:  "installed",
	}}
}

// syncGitPackage clones a git repository using gilt (via GitFetcher.Fetch),
// then looks for an agentpack.yaml. If found, it builds and installs. The
// ref from the user's config is used as the version — no .git/ needed.
func syncGitPackage(ctx context.Context, pkg Package) []Result {
	source := pkg.Git
	if pkg.Ref != "" {
		source = source + "#" + pkg.Ref
	}

	cloneDir, err := os.MkdirTemp("", "agentpack-sync-git-*")
	if err != nil {
		return []Result{{
			Name:   pkg.Name,
			Status: "failed",
			Err:    fmt.Errorf("create temp dir: %w", err),
		}}
	}
	defer func() { _ = os.RemoveAll(cloneDir) }()

	gf := &fetcher.GitFetcher{}
	if err := gf.Fetch(ctx, source, cloneDir); err != nil {
		return []Result{{
			Name:   pkg.Name,
			Status: "failed",
			Err:    fmt.Errorf("git fetch: %w", err),
		}}
	}

	vfs := osfs.NewWithNoIdm()
	buildResults, err := build.Run(ctx, vfs, build.Options{Dir: cloneDir})
	if err != nil {
		return []Result{{
			Name:   pkg.Name,
			Status: "failed",
			Err:    fmt.Errorf("build: %w", err),
		}}
	}

	var results []Result
	for _, br := range buildResults {
		r, installErr := install.Run(ctx, install.Options{
			Source: br.ArchivePath,
		})
		if installErr != nil {
			results = append(results, Result{
				Name:   br.Name,
				Status: "failed",
				Err:    installErr,
			})
			continue
		}
		results = append(results, Result{
			Name:    r.Name,
			Version: r.Version,
			Status:  "installed",
		})
	}

	return results
}
