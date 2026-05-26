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

// Package sync provides declarative plugin sync from agentpack-packages.yaml.
//
// Usage:
//
//	results, err := sync.Run(ctx, sync.Options{
//	    ConfigPath: "agentpack-packages.yaml",
//	    Builder:    sync.DefaultBuilder{},
//	    Installer:  sync.DefaultInstaller{},
//	    OnStep: func(name string) { fmt.Println("syncing", name) },
//	})
//
// Sync reads a packages YAML file and installs or updates every declared
// plugin. Git-sourced packages are cloned, built, and installed; URL or path
// sources are installed directly. Builder and Installer are injectable
// interfaces backed by DefaultBuilder and DefaultInstaller in production.
package sync

import (
	"context"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/retr0h/agentpack/pkg/fetcher"
)

// PackagesFile represents the top-level structure of agentpack-packages.yaml.
type PackagesFile struct {
	Packages []Package `yaml:"packages"`
}

// Package declares a single plugin source in agentpack-packages.yaml.
type Package struct {
	Name   string `yaml:"name"`
	Source string `yaml:"source"`
	Git    string `yaml:"git"`
	Ref    string `yaml:"ref"`
}

// Result holds the outcome for one package in a sync run.
type Result struct {
	Name    string
	Version string
	Status  string // "installed", "up to date", "failed"
	Err     error
}

// Options configures a sync run.
type Options struct {
	ConfigPath string
	Fetcher    fetcher.Fetcher // for git sources; nil uses default GitFetcher
	Builder    Builder         // for building from cloned repos; nil skips build
	Installer  Installer       // for installing archives; nil skips install

	// OnStep is called in real-time as each package is processed.
	// The name argument is the package name being synced.
	// When nil, no progress is reported.
	OnStep func(name string)
}

// Run reads configPath and installs or updates every declared package.
func Run(ctx context.Context, opts Options) ([]Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(opts.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", opts.ConfigPath, err)
	}

	var pf PackagesFile
	if err := yaml.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("parse %s: %w", opts.ConfigPath, err)
	}

	results := make([]Result, 0, len(pf.Packages))

	for _, pkg := range pf.Packages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if opts.OnStep != nil {
			opts.OnStep(pkg.Name)
		}

		var pkgResults []Result

		if pkg.Git != "" {
			pkgResults = syncGitPackage(ctx, pkg, opts)
		} else {
			pkgResults = syncSourcePackage(ctx, pkg, opts)
		}

		results = append(results, pkgResults...)
	}

	return results, nil
}

func syncSourcePackage(ctx context.Context, pkg Package, opts Options) []Result {
	if opts.Installer == nil {
		return []Result{{Name: pkg.Name, Status: "failed", Err: fmt.Errorf("no installer configured")}}
	}

	r, err := opts.Installer.Install(ctx, pkg.Source)
	if err != nil {
		return []Result{{Name: pkg.Name, Status: "failed", Err: err}}
	}

	return []Result{{Name: r.Name, Version: r.Version, Status: "installed"}}
}

func syncGitPackage(ctx context.Context, pkg Package, opts Options) []Result {
	source := pkg.Git
	if pkg.Ref != "" {
		source = source + "#" + pkg.Ref
	}

	f := opts.Fetcher
	if f == nil {
		f = &fetcher.GitFetcher{}
	}

	cloneDir, err := os.MkdirTemp("", "agentpack-sync-git-*")
	if err != nil {
		return []Result{{Name: pkg.Name, Status: "failed", Err: fmt.Errorf("create temp dir: %w", err)}}
	}
	defer func() { _ = os.RemoveAll(cloneDir) }()

	if err := f.Fetch(ctx, source, cloneDir); err != nil {
		return []Result{{Name: pkg.Name, Status: "failed", Err: fmt.Errorf("git fetch: %w", err)}}
	}

	if opts.Builder == nil {
		return []Result{{Name: pkg.Name, Status: "failed", Err: fmt.Errorf("no builder configured")}}
	}

	buildResults, err := opts.Builder.Build(ctx, cloneDir)
	if err != nil {
		return []Result{{Name: pkg.Name, Status: "failed", Err: fmt.Errorf("build: %w", err)}}
	}

	if opts.Installer == nil {
		return []Result{{Name: pkg.Name, Status: "failed", Err: fmt.Errorf("no installer configured")}}
	}

	var results []Result
	for _, br := range buildResults {
		r, err := opts.Installer.Install(ctx, br.ArchivePath)
		if err != nil {
			results = append(results, Result{Name: br.Name, Status: "failed", Err: err})
			continue
		}
		results = append(results, Result{Name: r.Name, Version: r.Version, Status: "installed"})
	}

	return results
}
