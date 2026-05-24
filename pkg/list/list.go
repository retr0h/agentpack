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

// Package list aggregates installed agentpack plugins across all registered
// targets.
package list

import (
	"fmt"
	"sort"

	"github.com/retr0h/agentpack/pkg/target"
)

// Entry represents a single installed agentpack plugin found by any target.
type Entry struct {
	Name      string
	Version   string
	SHA       string
	Installed string // build timestamp from metadata.json
	Dir       string // path to the installed plugin directory
	Target    string // which target (agent) it was found in
}

// Run queries each provided target for installed plugins and returns them
// aggregated and sorted by (Target, Name). When targets is nil or empty,
// target.All() is used so all registered targets are scanned.
func Run(targets []target.Target) ([]Entry, error) {
	if len(targets) == 0 {
		targets = target.All()
	}

	var entries []Entry

	for _, tgt := range targets {
		plugins, err := tgt.List()
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", tgt.Name(), err)
		}

		for _, p := range plugins {
			entries = append(entries, Entry{
				Name:      p.Name,
				Version:   p.Version,
				SHA:       p.SHA,
				Installed: p.Installed,
				Dir:       p.Dir,
				Target:    p.Target,
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Target != entries[j].Target {
			return entries[i].Target < entries[j].Target
		}

		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}
