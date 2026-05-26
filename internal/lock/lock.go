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
// SHAs for reproducible installs.
package lock

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LockedPackage records the resolved state of a single installed package.
type LockedPackage struct {
	// Name is the plugin identifier, matching the agentpack-packages.yaml entry.
	Name string `yaml:"name"`

	// Source is the git URL or archive path that was resolved.
	Source string `yaml:"source"`

	// Ref is the git ref that was requested (branch, tag, or SHA). Optional.
	Ref string `yaml:"ref,omitempty"`

	// SHA is the exact git commit SHA that was resolved and installed.
	SHA string `yaml:"sha"`

	// Resolved is the RFC3339 timestamp of when the package was resolved.
	Resolved string `yaml:"resolved"`
}

// Lockfile represents the full contents of an agentpack.lock file.
type Lockfile struct {
	// LockVersion is the schema version of this lockfile.
	LockVersion int `yaml:"lockVersion"`

	// Packages is the ordered list of resolved packages.
	Packages []LockedPackage `yaml:"packages"`
}

// Load reads the lockfile at path. When the file does not exist an empty
// Lockfile (with LockVersion 1) is returned without error, matching typical
// lockfile semantics.
func Load(path string) (*Lockfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Lockfile{LockVersion: 1}, nil
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

// Set inserts or replaces a LockedPackage entry. When an entry with the same
// Name already exists it is overwritten in place; otherwise it is appended.
func (lf *Lockfile) Set(p LockedPackage) {
	for i, existing := range lf.Packages {
		if existing.Name == p.Name {
			lf.Packages[i] = p

			return
		}
	}

	lf.Packages = append(lf.Packages, p)
}

// Remove deletes the LockedPackage with the given name. It is a no-op when
// the name does not exist.
func (lf *Lockfile) Remove(name string) {
	updated := lf.Packages[:0]
	for _, p := range lf.Packages {
		if p.Name != name {
			updated = append(updated, p)
		}
	}

	lf.Packages = updated
}

// Find returns a pointer to the LockedPackage with the given name, or nil
// when not found. The pointer references the slice element directly.
func (lf *Lockfile) Find(name string) *LockedPackage {
	for i := range lf.Packages {
		if lf.Packages[i].Name == name {
			return &lf.Packages[i]
		}
	}

	return nil
}
