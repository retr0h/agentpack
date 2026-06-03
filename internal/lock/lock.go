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

// Package lock manages the agentpack.lock file that pins resolved package
// state for reproducible installs.
package lock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/retr0h/agentpack/internal/merge"
)

// LockedFile records a single installed file with its integrity hash.
type LockedFile struct {
	Path   string `yaml:"path"   json:"path"`
	SHA256 string `yaml:"sha256" json:"sha256"`
	Target string `yaml:"target" json:"target"`
}

// LockedPackage records the resolved state of a single installed package.
type LockedPackage struct {
	Name     string       `yaml:"name"              json:"name"`
	Source   string       `yaml:"source"            json:"source"`
	Ref      string       `yaml:"ref,omitempty"     json:"ref,omitempty"`
	SHA      string       `yaml:"sha"               json:"sha"`
	Resolved string       `yaml:"resolved"          json:"resolved"`
	Content  []string     `yaml:"content,omitempty" json:"content,omitempty"`
	Skills   []string     `yaml:"skills,omitempty"  json:"skills,omitempty"`
	Targets  []string     `yaml:"targets,omitempty" json:"targets,omitempty"`
	Files    []LockedFile `yaml:"files,omitempty"   json:"files,omitempty"`
}

// Lockfile represents the full contents of an agentpack.lock file.
type Lockfile struct {
	LockVersion int             `yaml:"lockVersion" json:"lockVersion"`
	Packages    []LockedPackage `yaml:"packages"    json:"packages"`
}

// Load reads the lockfile at path. When the file does not exist an empty
// Lockfile (with LockVersion 2) is returned without error.
func Load(path string) (*Lockfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Lockfile{LockVersion: 2}, nil
		}

		return nil, fmt.Errorf("read lock file %s: %w", path, err)
	}

	var lf Lockfile
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("parse lock file %s: %w", path, err)
	}

	return &lf, nil
}

// Save serialises lf to path, creating parent directories as needed.
func Save(path string, lf *Lockfile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir for lock file: %w", err)
	}

	data, err := yaml.Marshal(lf)
	if err != nil {
		return fmt.Errorf("marshal lock file: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write lock file %s: %w", path, err)
	}

	return nil
}

// Set inserts or merges a LockedPackage entry. When an entry with the same
// Name exists, files are merged (new files replace existing by path+target,
// new files are appended), and skills/targets are merged and deduplicated.
func (lf *Lockfile) Set(p LockedPackage) {
	for i, existing := range lf.Packages {
		if existing.Name == p.Name {
			p.Files = mergeLockedFiles(existing.Files, p.Files)
			p.Content = merge.Strings(existing.Content, p.Content)
			p.Skills = merge.Strings(existing.Skills, p.Skills)
			p.Targets = merge.Strings(existing.Targets, p.Targets)
			lf.Packages[i] = p

			return
		}
	}

	lf.Packages = append(lf.Packages, p)
}

// Remove deletes the LockedPackage with the given name.
func (lf *Lockfile) Remove(name string) {
	updated := lf.Packages[:0]
	for _, p := range lf.Packages {
		if p.Name != name {
			updated = append(updated, p)
		}
	}

	lf.Packages = updated
}

// RemoveContent removes content entries from an existing package and prunes
// matching files. Each entry in contentStrs is a "type/name" selector (e.g.
// "skill/k8s"). Per ADR-010 this replaces the old RemoveSkill method.
func (lf *Lockfile) RemoveContent(name string, contentStrs []string) {
	p := lf.Find(name)
	if p == nil {
		return
	}

	removeSet := make(map[string]bool, len(contentStrs))
	for _, c := range contentStrs {
		removeSet[c] = true
	}

	// Prune the content list.
	remaining := make([]string, 0, len(p.Content))
	for _, c := range p.Content {
		if !removeSet[c] {
			remaining = append(remaining, c)
		}
	}

	p.Content = remaining

	// Prune files matching any removed content selector.
	var keptFiles []LockedFile
	for _, f := range p.Files {
		keep := true
		for _, c := range contentStrs {
			if fileMatchesContent(f.Path, c) {
				keep = false
				break
			}
		}

		if keep {
			keptFiles = append(keptFiles, f)
		}
	}

	p.Files = keptFiles
}

// RemoveSkill removes a skill from an existing entry's Skills list and
// prunes any files containing that skill name in their path.
// Deprecated: use RemoveContent instead. Maintained for backward compatibility.
func (lf *Lockfile) RemoveSkill(name, skill string) {
	p := lf.Find(name)
	if p == nil {
		return
	}

	remaining := make([]string, 0, len(p.Skills))
	for _, s := range p.Skills {
		if s != skill {
			remaining = append(remaining, s)
		}
	}

	p.Skills = remaining

	var keptFiles []LockedFile
	for _, f := range p.Files {
		if !fileMatchesSkill(f.Path, skill) {
			keptFiles = append(keptFiles, f)
		}
	}

	p.Files = keptFiles
}

// Find returns a pointer to the LockedPackage with the given name, or nil.
func (lf *Lockfile) Find(name string) *LockedPackage {
	for i := range lf.Packages {
		if lf.Packages[i].Name == name {
			return &lf.Packages[i]
		}
	}

	return nil
}

// fileMatchesContent checks whether a file path matches a "type/name" content
// selector. The type maps to a directory (e.g. "skill" -> "skills", "command"
// -> "commands") and the name is matched as a path component.
func fileMatchesContent(path, content string) bool {
	parts := strings.SplitN(content, "/", 2)
	if len(parts) != 2 {
		return false
	}

	contentType, contentName := parts[0], parts[1]

	// Map content type to directory name.
	var dirName string
	switch contentType {
	case "skill":
		dirName = "skills"
	case "command":
		dirName = "commands"
	case "hook":
		dirName = "hooks"
	case "agent":
		dirName = "agents"
	case "mcp":
		dirName = "mcp"
	case "config":
		dirName = "settings"
	default:
		return false
	}

	normalized := filepath.ToSlash(path)

	// Match patterns like /skills/name/ or skills/name/
	midNeedle := "/" + dirName + "/" + contentName + "/"
	leadNeedle := dirName + "/" + contentName + "/"

	return strings.Contains(normalized, midNeedle) ||
		strings.HasPrefix(normalized, leadNeedle)
}

func fileMatchesSkill(path, skill string) bool {
	slashed := filepath.ToSlash(path)

	return slashed != "" &&
		(strings.Contains(slashed, "/skills/"+skill+"/") ||
			strings.Contains(slashed, "/skills/"+skill))
}

func mergeLockedFiles(existing, incoming []LockedFile) []LockedFile {
	type key struct {
		Path   string
		Target string
	}

	seen := make(map[key]int, len(existing))
	merged := make([]LockedFile, len(existing))
	copy(merged, existing)

	for i, f := range merged {
		seen[key{f.Path, f.Target}] = i
	}

	for _, f := range incoming {
		k := key{f.Path, f.Target}
		if idx, ok := seen[k]; ok {
			merged[idx] = f
		} else {
			seen[k] = len(merged)
			merged = append(merged, f)
		}
	}

	return merged
}
