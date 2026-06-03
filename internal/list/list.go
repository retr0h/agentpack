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
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/retr0h/agentpack/internal/cli"
	"github.com/retr0h/agentpack/internal/driver/agents"
	"github.com/retr0h/agentpack/internal/gitutil"
	"github.com/retr0h/agentpack/internal/registry"
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

// Status describes the on-disk health of an installed package.
type Status string

// Status constants for on-disk health.
const (
	StatusOK      Status = "ok"
	StatusMissing Status = "missing"
	StatusEmpty   Status = "empty"
)

// TargetInfo holds a target name and the number of files installed for it.
type TargetInfo struct {
	Name      string `json:"name"`
	FileCount int    `json:"fileCount"`
}

// ContentItem holds a content type/name pair and which targets it was installed to.
type ContentItem struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Targets []string `json:"targets"`
}

// Entry represents a single installed package.
type Entry struct {
	Name           string         `json:"name"`
	Version        string         `json:"version"`
	SHA            string         `json:"sha"`
	Source         string         `json:"source"`
	Targets        []TargetInfo   `json:"targets"`
	Contents       []ContentItem  `json:"contents"`
	SelectedSkills []string       `json:"selectedSkills,omitempty"`
	Installed      string         `json:"installed"`
	Scope          registry.Scope `json:"scope"`
	Status         Status         `json:"status"`
}

// GlobalEntry represents a single globally installed skill.
type GlobalEntry struct {
	Agent string `json:"agent"`
	Skill string `json:"skill"`
	Dir   string `json:"dir"`
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

	all, err := reg.List()
	if err != nil {
		return nil, err
	}

	// Filter out manifests that have no tracked files — they are registry
	// artefacts from installs that produced nothing (e.g. content-less repos).
	manifests := make([]*registry.PackageManifest, 0, len(all))
	for _, m := range all {
		if len(m.Files) > 0 {
			manifests = append(manifests, m)
		}
	}

	var entries []Entry

	for _, m := range manifests {
		targets := collectTargets(m)

		var status Status
		if len(m.Files) == 0 {
			status = StatusEmpty
		} else {
			found := false
			for _, f := range m.Files {
				path := filepath.Join(f.Dir, f.Path)
				if _, err := os.Stat(path); err == nil {
					found = true

					break
				}
			}

			if found {
				status = StatusOK
			} else {
				status = StatusMissing
			}
		}

		scope := m.Scope
		if scope == "" {
			scope = registry.ScopeLocal
		}

		entries = append(entries, Entry{
			Name:           m.Name,
			Version:        shortVersion(m.Version),
			SHA:            gitutil.ShortSHA(m.SHA),
			Source:         shortSource(m.Source),
			Targets:        targets,
			Contents:       extractContentItems(m),
			SelectedSkills: m.SelectedSkills,
			Installed:      cli.FormatDate(m.Installed),
			Scope:          scope,
			Status:         status,
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
		if errors.Is(readErr, os.ErrNotExist) {
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

func collectTargets(m *registry.PackageManifest) []TargetInfo {
	counts := make(map[string]int)

	for _, f := range m.Files {
		counts[f.Target]++
	}

	targets := make([]string, 0, len(counts))
	for t := range counts {
		targets = append(targets, t)
	}

	slices.Sort(targets)

	infos := make([]TargetInfo, len(targets))
	for i, t := range targets {
		infos[i] = TargetInfo{Name: t, FileCount: counts[t]}
	}

	return infos
}

// extractContentItems infers content type and name from archive file paths by
// scanning for a known content-type directory segment followed by an item name
// segment. This path-based inference works because the archive layout (ADR-009)
// preserves conventional directories — skills/, commands/, hooks/, agents/,
// mcp/, settings/ — even as metadata becomes the authoritative type contract.
// Both "settings" and "config" are recognised: "settings" is the legacy
// directory name from ADR-001; "config" is the canonical type introduced in
// ADR-009.
var contentDirs = map[string]bool{
	"skills": true, "commands": true, "agents": true,
	"hooks": true, "mcp": true, "settings": true, "config": true,
}

func extractContentItems(m *registry.PackageManifest) []ContentItem {
	type itemKey struct{ typ, name string }

	itemTargets := make(map[itemKey]map[string]bool)

	for _, f := range m.Files {
		normalized := filepath.ToSlash(f.Path)
		parts := strings.Split(normalized, "/")

		for i, part := range parts {
			if !contentDirs[part] || i+1 >= len(parts) {
				continue
			}

			itemName := parts[i+1]

			if part == "skills" && i > 0 && parts[i-1] == ".agents" && i+2 < len(parts) {
				itemName = parts[i+2]
			}

			if strings.Contains(itemName, ".") {
				itemName = strings.TrimSuffix(itemName, filepath.Ext(itemName))
			}

			if itemName == "" {
				continue
			}

			k := itemKey{part, itemName}
			if itemTargets[k] == nil {
				itemTargets[k] = make(map[string]bool)
			}

			itemTargets[k][f.Target] = true
		}
	}

	items := make([]ContentItem, 0, len(itemTargets))
	for k, tgts := range itemTargets {
		targets := make([]string, 0, len(tgts))
		for t := range tgts {
			targets = append(targets, t)
		}

		slices.Sort(targets)
		items = append(items, ContentItem{Type: k.typ, Name: k.name, Targets: targets})
	}

	slices.SortFunc(items, func(a, b ContentItem) int {
		if a.Type != b.Type {
			return cmp.Compare(a.Type, b.Type)
		}
		return cmp.Compare(a.Name, b.Name)
	})

	return items
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
