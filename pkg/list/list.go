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
//
// Usage:
//
//	l := list.New()
//	entries, err := l.Run()
//
// Run returns all installed plugins sorted by name. The Registry parameter
// in RunWithRegistry accepts nil, in which case the production registry
// implementation is used. Pass a custom Registry to inject a test double.
package list

import (
	"cmp"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/retr0h/agentpack/pkg/registry"
	"github.com/retr0h/agentpack/pkg/target/agents"
)

// Registry lists all installed package manifests from the registry store.
// Implement this interface to inject a test double in place of registry.List.
type Registry interface {
	List() ([]*registry.PackageManifest, error)
}

// defaultRegistry wraps registry.New().List to satisfy Registry.
type defaultRegistry struct{}

func (defaultRegistry) List() ([]*registry.PackageManifest, error) {
	return registry.New().List()
}

// Entry represents a single installed package.
type Entry struct {
	Name      string
	Version   string
	SHA       string
	Source    string
	Targets   string
	Installed string
}

// GlobalEntry represents a single globally installed skill.
type GlobalEntry struct {
	Agent string
	Skill string
	Dir   string
}

// Lister reads installed packages from the registry.
type Lister struct{}

// New returns a new Lister.
func New() *Lister { return &Lister{} }

// Run reads from the registry and returns all installed packages sorted by name.
// It uses the production registry.List implementation.
func (l *Lister) Run() ([]Entry, error) {
	return l.RunWithRegistry(nil)
}

// RunWithRegistry is like Run but allows injecting a custom Registry
// implementation for testing.
func (l *Lister) RunWithRegistry(reg Registry) ([]Entry, error) {
	if reg == nil {
		reg = defaultRegistry{}
	}

	manifests, err := reg.List()
	if err != nil {
		return nil, err
	}

	var entries []Entry

	for _, m := range manifests {
		targets := collectTargets(m)

		entries = append(entries, Entry{
			Name:      m.Name,
			Version:   shortVersion(m.Version),
			SHA:       shortSHA(m.SHA),
			Source:    shortSource(m.Source),
			Targets:   strings.Join(targets, ", "),
			Installed: formatDate(m.Installed),
		})
	}

	slices.SortFunc(entries, func(a, b Entry) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return entries, nil
}

// osUserHomeDir is a swappable wrapper around os.UserHomeDir so tests can
// redirect the home directory without t.Setenv (which is incompatible with
// t.Parallel).
var osUserHomeDir = os.UserHomeDir

// RunGlobal scans each agent's GlobalSkillsDir under the user's home directory
// and returns one entry per discovered skill subdirectory.
func (l *Lister) RunGlobal() ([]GlobalEntry, error) {
	home, err := osUserHomeDir()
	if err != nil {
		return nil, err
	}

	var entries []GlobalEntry

	for _, def := range agents.Defs() {
		if def.GlobalSkillsDir == "" {
			continue
		}

		skillsDir := filepath.Join(home, def.GlobalSkillsDir)

		dirEntries, readErr := os.ReadDir(skillsDir)
		if os.IsNotExist(readErr) {
			continue
		}

		if readErr != nil {
			return nil, readErr
		}

		for _, de := range dirEntries {
			if de.IsDir() {
				entries = append(entries, GlobalEntry{
					Agent: def.Name,
					Skill: de.Name(),
					Dir:   skillsDir,
				})
			}
		}
	}

	return entries, nil
}

func collectTargets(m *registry.PackageManifest) []string {
	seen := make(map[string]bool)

	for _, f := range m.Files {
		if !seen[f.Target] {
			seen[f.Target] = true
		}
	}

	targets := make([]string, 0, len(seen))
	for t := range seen {
		targets = append(targets, t)
	}

	slices.Sort(targets)

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

func shortVersion(v string) string {
	if looksLikeSHA(v) {
		return v[:7]
	}

	return v
}

func shortSource(s string) string {
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")

	if idx := strings.IndexByte(s, '#'); idx >= 0 {
		s = s[:idx]
	}

	return s
}

func looksLikeSHA(s string) bool {
	if len(s) < 40 {
		return false
	}

	_, err := hex.DecodeString(s[:40])

	return err == nil
}
