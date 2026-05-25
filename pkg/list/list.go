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

// Package list reads from the registry to show installed packages.
package list

import (
	"sort"
	"strings"

	"github.com/retr0h/agentpack/pkg/registry"
)

// Entry represents a single installed package.
type Entry struct {
	Name      string
	Version   string
	SHA       string
	Source    string
	Targets   string
	Installed string
}

// Run reads from the registry and returns all installed packages.
func Run() ([]Entry, error) {
	manifests, err := registry.List()
	if err != nil {
		return nil, err
	}

	var entries []Entry

	for _, m := range manifests {
		targets := collectTargets(m)

		entries = append(entries, Entry{
			Name:      m.Name,
			Version:   m.Version,
			SHA:       shortSHA(m.SHA),
			Source:    m.Source,
			Targets:   strings.Join(targets, ", "),
			Installed: formatDate(m.Installed),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}

func collectTargets(m *registry.PackageManifest) []string {
	seen := make(map[string]bool)

	for _, f := range m.Files {
		if !seen[f.Target] {
			seen[f.Target] = true
		}
	}

	var targets []string
	for t := range seen {
		targets = append(targets, t)
	}

	sort.Strings(targets)

	return targets
}

func shortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}

	return sha
}

func formatDate(ts string) string {
	if idx := strings.IndexByte(ts, 'T'); idx > 0 {
		return ts[:idx]
	}

	return ts
}
