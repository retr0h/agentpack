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

// Package lockfile manages the agentpack-lock.yaml file that records installed
// plugins for project-local and global installs.
package lockfile

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Install describes a single installed plugin entry in the lockfile.
type Install struct {
	// Name is the plugin identifier.
	Name string `yaml:"name"`

	// Source is the original install source (URL, path, or git ref).
	Source string `yaml:"source"`

	// Ref is the git ref that was requested (branch, tag, or SHA).
	Ref string `yaml:"ref,omitempty"`

	// Version is the plugin version from its manifest.
	Version string `yaml:"version,omitempty"`

	// Targets lists the target names the plugin was installed into.
	Targets []string `yaml:"targets,omitempty"`

	// Enabled controls whether the plugin is active.
	Enabled bool `yaml:"enabled"`

	// Installed is the RFC3339 timestamp of when the plugin was installed.
	Installed string `yaml:"installed,omitempty"`
}

// Lockfile represents the full contents of an agentpack-lock.yaml file.
type Lockfile struct {
	Installs []Install `yaml:"installs"`
}

// Read parses the lockfile at path. It returns an empty Lockfile (not an
// error) when the file does not exist, matching typical lockfile semantics.
func Read(path string) (*Lockfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Lockfile{}, nil
		}

		return nil, fmt.Errorf("read lockfile %s: %w", path, err)
	}

	var lf Lockfile
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("parse lockfile %s: %w", path, err)
	}

	return &lf, nil
}

// Write serialises lf to path, creating parent directories as needed.
func Write(path string, lf *Lockfile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir for lockfile: %w", err)
	}

	data, err := yaml.Marshal(lf)
	if err != nil {
		return fmt.Errorf("marshal lockfile: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write lockfile %s: %w", path, err)
	}

	return nil
}

// Add inserts or updates an Install entry in the lockfile. When an entry with
// the same Name already exists, its Targets list is merged with the incoming
// entry's Targets (deduplication). Other fields are overwritten.
func (lf *Lockfile) Add(entry Install) {
	for i, e := range lf.Installs {
		if e.Name == entry.Name {
			// Merge targets.
			merged := mergeTargets(e.Targets, entry.Targets)
			entry.Targets = merged
			lf.Installs[i] = entry

			return
		}
	}

	lf.Installs = append(lf.Installs, entry)
}

// Remove deletes the Install entry with the given name, if present.
func (lf *Lockfile) Remove(name string) {
	updated := lf.Installs[:0]
	for _, e := range lf.Installs {
		if e.Name != name {
			updated = append(updated, e)
		}
	}

	lf.Installs = updated
}

// Find returns a pointer to the Install entry with the given name, or nil
// when not found. The pointer references the slice element directly.
func (lf *Lockfile) Find(name string) *Install {
	for i := range lf.Installs {
		if lf.Installs[i].Name == name {
			return &lf.Installs[i]
		}
	}

	return nil
}

// SetEnabled sets the Enabled field for the named entry. It returns true when
// the entry was found and updated, false when the name does not exist.
func (lf *Lockfile) SetEnabled(name string, enabled bool) bool {
	for i := range lf.Installs {
		if lf.Installs[i].Name == name {
			lf.Installs[i].Enabled = enabled

			return true
		}
	}

	return false
}

// mergeTargets returns a deduplicated union of a and b.
func mergeTargets(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))

	for _, t := range a {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}

	for _, t := range b {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}

	return out
}
