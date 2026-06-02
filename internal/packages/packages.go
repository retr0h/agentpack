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

// Package packages manages the agentpack-packages.yaml spec file that declares
// which plugins a project wants installed.
package packages

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Package declares a single plugin entry in the spec file.
type Package struct {
	// Name is the plugin identifier.
	Name string `yaml:"name"`

	// Git is the git repository URL (e.g. github.com/org/repo). Set when the
	// source is a hosted git repository.
	Git string `yaml:"git,omitempty"`

	// Ref is the git ref (branch, tag, or SHA) to pin. Optional.
	Ref string `yaml:"ref,omitempty"`

	// Source is the local path or HTTP URL to a .agentpack archive. Set when
	// the source is not a git repository.
	Source string `yaml:"source,omitempty"`

	// Skills restricts the install to named skills. Optional.
	Skills []string `yaml:"skills,omitempty"`

	// Targets restricts which agent targets to install to. Optional.
	Targets []string `yaml:"targets,omitempty"`
}

// gitHosts lists the domain fragments that indicate a git-hosted source.
var gitHosts = []string{"github.com", "gitlab.com", "bitbucket.org"}

// BuildFromSource constructs a Package from the install source URL.
// Git-hosted sources populate the Git (and optionally Ref) fields; everything
// else populates the Source field.
func BuildFromSource(name, source string, skills, targets []string) Package {
	pkg := Package{Name: name}

	isGit := false

	for _, host := range gitHosts {
		if strings.Contains(source, host) {
			isGit = true

			break
		}
	}

	if isGit {
		gitURL := source
		ref := ""

		if idx := strings.LastIndex(gitURL, "#"); idx >= 0 {
			ref = gitURL[idx+1:]
			gitURL = gitURL[:idx]
		}

		pkg.Git = gitURL
		pkg.Ref = ref
	} else {
		pkg.Source = source
	}

	if len(skills) > 0 {
		pkg.Skills = skills
	}

	if len(targets) > 0 {
		pkg.Targets = targets
	}

	return pkg
}

// Config is the parsed contents of agentpack-packages.yaml.
type Config struct {
	Packages []Package `yaml:"packages"`
}

// Load reads the config file at path. When the file does not exist an empty
// Config is returned without error, matching typical manifest semantics.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}

		return nil, fmt.Errorf("read packages file %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse packages file %s: %w", path, err)
	}

	return &cfg, nil
}

// Save serialises cfg to path, creating parent directories as needed.
func Save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir for packages file: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal packages file: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write packages file %s: %w", path, err)
	}

	return nil
}

// Add inserts or replaces a Package entry. When an entry with the same Name
// already exists it is overwritten in place; otherwise it is appended.
func (c *Config) Add(p Package) {
	for i, existing := range c.Packages {
		if existing.Name == p.Name {
			p.Skills = mergeStrings(existing.Skills, p.Skills)
			p.Targets = mergeStrings(existing.Targets, p.Targets)
			c.Packages[i] = p

			return
		}
	}

	c.Packages = append(c.Packages, p)
}

func mergeStrings(a, b []string) []string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(a)+len(b))
	for _, s := range a {
		seen[s] = true
	}
	for _, s := range b {
		seen[s] = true
	}

	result := make([]string, 0, len(seen))
	for s := range seen {
		result = append(result, s)
	}

	sort.Strings(result)

	return result
}

// Remove deletes the Package with the given name. It is a no-op when the name
// does not exist.
func (c *Config) Remove(name string) {
	updated := c.Packages[:0]
	for _, p := range c.Packages {
		if p.Name != name {
			updated = append(updated, p)
		}
	}

	c.Packages = updated
}

// Find returns a pointer to the Package with the given name, or nil when not
// found. The pointer references the slice element directly.
func (c *Config) Find(name string) *Package {
	for i := range c.Packages {
		if c.Packages[i].Name == name {
			return &c.Packages[i]
		}
	}

	return nil
}
