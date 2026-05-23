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

// Package sync provides declarative plugin sync from a claudia-packages.yaml.
package sync

import (
	"context"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/retr0h/claudia/pkg/install"
)

// PackagesFile represents the top-level structure of claudia-packages.yaml.
type PackagesFile struct {
	Packages []Package `yaml:"packages"`
}

// Package declares a single plugin source in claudia-packages.yaml.
type Package struct {
	Name   string `yaml:"name"`
	Source string `yaml:"source"`
}

// Result holds the outcome for one package in a sync run.
type Result struct {
	Name    string
	Version string
	Status  string // "installed", "up to date", "failed"
	Err     error
}

// Run reads configPath (a claudia-packages.yaml) and installs or updates every
// declared package into pluginDir. Results are returned for every package,
// including failures. A non-nil error is returned only when the config file
// itself cannot be read or parsed.
func Run(ctx context.Context, configPath string, pluginDir string) ([]Result, error) {
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

		r, installErr := install.Run(ctx, install.Options{
			Source:    pkg.Source,
			PluginDir: pluginDir,
		})

		if installErr != nil {
			results = append(results, Result{
				Name:   pkg.Name,
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

	return results, nil
}
