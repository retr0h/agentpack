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

// Package remove safely uninstalls an agentpack plugin by deleting only the
// exact files that were recorded in its registry manifest.
//
// Usage:
//
//	r := remove.New()
//	result, err := r.Run(ctx, remove.Options{
//	    Name: "my-plugin",
//	})
//
// SAFETY INVARIANT: Remover never walks directories and never deletes any path
// that contains ".git". Every deletion is guarded by:
//  1. The path comes from the registry manifest (explicit list).
//  2. The path does NOT contain ".git".
//  3. The file SHA256 matches the recorded checksum (skipped if modified).
//
// The Registry field in Options accepts nil, in which case the production
// registry implementation is used.
package remove

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/retr0h/agentpack/pkg/registry"
)

// Registry loads and removes package manifests from the registry store.
// Implement this interface to inject a test double in place of the default
// registry.Load / registry.Remove functions.
type Registry interface {
	Load(name string) (*registry.PackageManifest, error)
	Remove(name string) error
}

// Step represents a completed remove action for real-time display.
type Step struct {
	// Path is the file path that was acted on.
	Path string

	// Skipped is true when the file was not removed (modified or protected).
	Skipped bool
}

// Options configures a remove run.
type Options struct {
	// Name is the plugin identifier to remove.
	Name string

	// OnStep is called in real-time as each file is removed or skipped.
	// When nil, no progress is reported.
	OnStep func(Step)

	// Registry overrides the registry backend. When nil the default
	// registry.Load / registry.Remove implementation is used.
	Registry Registry
}

// RemovedFile records a file that was successfully removed.
type RemovedFile struct {
	// Path is the absolute path that was removed.
	Path string

	// Skipped is true when the file was not removed because its checksum
	// no longer matched the recorded value (user-modified).
	Skipped bool
}

// Result holds the outcome of a remove run.
type Result struct {
	// Name is the plugin that was removed.
	Name string

	// Removed lists files that were successfully deleted.
	Removed []RemovedFile

	// Skipped lists files that were not deleted because they were modified.
	Skipped []RemovedFile
}

// defaultRegistry wraps registry.New() to satisfy Registry.
type defaultRegistry struct{}

func (defaultRegistry) Load(name string) (*registry.PackageManifest, error) {
	return registry.New().Load(name)
}

func (defaultRegistry) Remove(name string) error {
	return registry.New().Remove(name)
}

// Remover safely uninstalls plugins using their registry manifest.
type Remover struct{}

// New returns a new Remover.
func New() *Remover { return &Remover{} }

// Run removes the named plugin using the registry manifest to determine
// exactly which files to delete. It never walks directories.
func (r *Remover) Run(ctx context.Context, opts Options) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	reg := opts.Registry
	if reg == nil {
		reg = defaultRegistry{}
	}

	// Load the registry manifest for this package.
	m, err := reg.Load(opts.Name)
	if err != nil {
		return nil, fmt.Errorf("load registry manifest: %w", err)
	}

	result := &Result{Name: opts.Name}

	for _, f := range m.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		absPath := filepath.Join(f.Dir, f.Path)

		// SAFETY: never delete anything under .git/.
		if containsGit(absPath) {
			result.Skipped = append(result.Skipped, RemovedFile{Path: absPath, Skipped: true})
			emitStep(opts, Step{Path: absPath, Skipped: true})

			continue
		}

		// Verify the file checksum matches before removing.
		ok, checksumErr := matchesChecksum(absPath, f.SHA256)
		if checksumErr != nil || !ok {
			// File is missing or user-modified; skip it.
			result.Skipped = append(result.Skipped, RemovedFile{Path: absPath, Skipped: true})
			emitStep(opts, Step{Path: absPath, Skipped: true})

			continue
		}

		if removeErr := os.Remove(absPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return nil, fmt.Errorf("remove %s: %w", absPath, removeErr)
		}

		result.Removed = append(result.Removed, RemovedFile{Path: absPath})
		emitStep(opts, Step{Path: absPath, Skipped: false})
	}

	// Remove the registry manifest.
	if err := reg.Remove(opts.Name); err != nil {
		return nil, fmt.Errorf("remove registry entry: %w", err)
	}

	return result, nil
}

// emitStep calls opts.OnStep when the callback is set.
func emitStep(opts Options, s Step) {
	if opts.OnStep != nil {
		opts.OnStep(s)
	}
}

// containsGit returns true when path contains a ".git" component, which would
// indicate an attempt to delete version-control internals.
func containsGit(path string) bool {
	clean := filepath.Clean(path)

	return slices.Contains(
		strings.Split(clean, string(filepath.Separator)),
		".git",
	)
}

// matchesChecksum returns true when the file at path has the expected SHA256.
func matchesChecksum(path string, expected string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}

	got := hex.EncodeToString(h.Sum(nil))

	return got == expected, nil
}
