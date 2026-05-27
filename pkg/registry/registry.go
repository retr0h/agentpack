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

// Package registry manages per-package installation manifests stored at
// ~/.config/agentpack/packages/{name}.yaml. Each manifest records exactly
// which files were installed and their SHA256 checksums so that remove can
// safely undo an installation without walking directories.
//
// Usage:
//
//	r := registry.New()
//
//	// Save an installation manifest.
//	err := r.Save(&registry.PackageManifest{
//	    Name:    "my-plugin",
//	    Source:  "github.com/org/repo",
//	    Files:   installedFiles,
//	})
//
//	// Load a manifest by name.
//	m, err := r.Load("my-plugin")
//
//	// List all installed manifests.
//	manifests, err := r.List()
//
//	// Remove a manifest.
//	err = r.Remove("my-plugin")
package registry

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Scope identifies whether a package was installed into a project-local or
// user-global location.
type Scope string

// Scope constants for install location.
const (
	ScopeLocal  Scope = "local"
	ScopeGlobal Scope = "global"
)

// osUserHomeDir is a swappable wrapper around os.UserHomeDir so tests can
// redirect registry writes to a temp directory without touching the real
// ~/.config/agentpack/packages/ tree.
var osUserHomeDir = os.UserHomeDir

// SetOsUserHomeDir replaces the home-dir lookup function used by Dir (and
// transitively by Save, Load, Remove, and List). It returns a restore
// function that callers should defer so each test cleans up after itself.
// This is intended for tests in other packages (e.g. list, install) that
// write through the registry and must not pollute the real home directory.
func SetOsUserHomeDir(fn func() (string, error)) func() {
	orig := osUserHomeDir
	osUserHomeDir = fn

	return func() { osUserHomeDir = orig }
}

// InstalledFile records a single file placed on disk during an install.
type InstalledFile struct {
	// Path is the relative path of the file within its target directory.
	Path string `yaml:"path" json:"path"`

	// SHA256 is the hex-encoded SHA256 of the file at install time.
	SHA256 string `yaml:"sha256" json:"sha256"`

	// Target is the name of the target driver that wrote this file
	// (e.g. "claude-code", "cursor").
	Target string `yaml:"target" json:"target"`

	// Dir is the absolute path to the directory that contains Path.
	Dir string `yaml:"dir" json:"dir"`
}

// PackageManifest records everything needed to manage (update/remove) an
// installed package.
type PackageManifest struct {
	// Name is the plugin identifier.
	Name string `yaml:"name" json:"name"`

	// Source is the original install source URI.
	Source string `yaml:"source" json:"source"`

	// Ref is the git ref that was resolved during install.
	Ref string `yaml:"ref,omitempty" json:"ref,omitempty"`

	// SHA is the resolved git commit SHA.
	SHA string `yaml:"sha,omitempty" json:"sha,omitempty"`

	// Version is the plugin version string.
	Version string `yaml:"version,omitempty" json:"version,omitempty"`

	// Installed is the RFC3339 timestamp of when the package was installed.
	Installed string `yaml:"installed,omitempty" json:"installed,omitempty"`

	// Scope is ScopeLocal or ScopeGlobal.
	Scope Scope `yaml:"scope,omitempty" json:"scope,omitempty"`

	// Files is the complete list of files written to disk.
	Files []InstalledFile `yaml:"files" json:"files"`
}

// Registry manages per-package installation manifests.
type Registry struct{}

// New returns a new Registry ready to manage installation manifests.
func New() *Registry { return &Registry{} }

// Dir returns the path to the package registry directory, creating it if it
// does not exist. The directory is ~/.config/agentpack/packages/.
func (r *Registry) Dir() (string, error) {
	home, err := osUserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}

	dir := filepath.Join(home, ".config", "agentpack", "packages")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir registry dir: %w", err)
	}

	return dir, nil
}

// Save writes m to the registry directory. The file is named {m.Name}.yaml.
func (r *Registry) Save(m *PackageManifest) error {
	dir, err := r.Dir()
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	path := filepath.Join(dir, m.Name+".yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write manifest %s: %w", path, err)
	}

	return nil
}

// Load reads the registry manifest for name. It returns an error when the
// manifest does not exist.
func (r *Registry) Load(name string) (*PackageManifest, error) {
	dir, err := r.Dir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, name+".yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("package %q not found in registry", name)
		}

		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}

	var m PackageManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}

	return &m, nil
}

// Remove deletes the registry manifest for name. It is a no-op when the
// manifest does not exist.
func (r *Registry) Remove(name string) error {
	dir, err := r.Dir()
	if err != nil {
		return err
	}

	path := filepath.Join(dir, name+".yaml")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove manifest %s: %w", path, err)
	}

	return nil
}

// List returns all PackageManifests stored in the registry directory.
func (r *Registry) List() ([]*PackageManifest, error) {
	dir, err := r.Dir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("read registry dir: %w", err)
	}

	var manifests []*PackageManifest

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}

		name := e.Name()[:len(e.Name())-len(".yaml")]

		m, loadErr := r.Load(name)
		if loadErr != nil {
			return nil, loadErr
		}

		manifests = append(manifests, m)
	}

	return manifests, nil
}
