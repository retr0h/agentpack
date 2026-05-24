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

// Package manifest handles agentpack.yaml parsing and validation.
package manifest

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/avfs/avfs"
	"gopkg.in/yaml.v3"
)

// Manifest is the top-level structure of a agentpack.yaml file. It supports both
// single-plugin (name/version/description at the top level) and multi-plugin
// (plugins[] array) forms. The two forms are mutually exclusive.
type Manifest struct {
	Author   Author `yaml:"author"`
	License  string `yaml:"license"`
	Homepage string `yaml:"homepage"`

	Name        string     `yaml:"name"`
	Version     string     `yaml:"version"`
	Description string     `yaml:"description"`
	Keywords    []string   `yaml:"keywords"`
	Category    string     `yaml:"category"`
	Skills      []Entry    `yaml:"skills"`
	Commands    []Entry    `yaml:"commands"`
	Hooks       []Entry    `yaml:"hooks"`
	Agents      []Entry    `yaml:"agents"`
	MCP         []MCPEntry `yaml:"mcp"`
	Binaries    []Entry    `yaml:"binaries"`
	Settings    []Entry    `yaml:"settings"`

	Plugins []Plugin `yaml:"plugins"`
}

// Plugin represents a single plugin entry in the multi-plugin form of
// agentpack.yaml.
type Plugin struct {
	Name        string     `yaml:"name"`
	Version     string     `yaml:"version"`
	Description string     `yaml:"description"`
	Author      Author     `yaml:"author"`
	License     string     `yaml:"license"`
	Homepage    string     `yaml:"homepage"`
	Keywords    []string   `yaml:"keywords"`
	Category    string     `yaml:"category"`
	Skills      []Entry    `yaml:"skills"`
	Commands    []Entry    `yaml:"commands"`
	Hooks       []Entry    `yaml:"hooks"`
	Agents      []Entry    `yaml:"agents"`
	MCP         []MCPEntry `yaml:"mcp"`
	Binaries    []Entry    `yaml:"binaries"`
	Settings    []Entry    `yaml:"settings"`
}

// Author holds attribution information for a plugin or manifest.
type Author struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
}

// MCPEntry describes a Model Context Protocol server bundled with a plugin.
type MCPEntry struct {
	Type      string            `yaml:"type"`
	Name      string            `yaml:"name"`
	Src       string            `yaml:"src"`
	Config    string            `yaml:"config"`
	URL       string            `yaml:"url"`
	Package   string            `yaml:"package"`
	Args      []string          `yaml:"args"`
	Env       map[string]string `yaml:"env"`
	Platforms []string          `yaml:"platforms"`
}

// Entry describes a file or glob that is part of a plugin. A bare YAML string
// is treated as a glob pattern; an object with src/dest fields describes an
// explicit file mapping.
type Entry struct {
	Glob string `yaml:"-"`
	Src  string `yaml:"src"`
	Dest string `yaml:"dest"`
}

// UnmarshalYAML implements yaml.Unmarshaler. A scalar YAML node is treated as
// a glob pattern stored in Glob; an object node is decoded into Src and Dest.
func (e *Entry) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		e.Glob = value.Value
		return nil
	}
	type raw Entry
	return value.Decode((*raw)(e))
}

// Load reads agentpack.yaml from dir using vfs, unmarshals it, and validates it.
// It returns an error when the file is missing, malformed, or fails
// validation.
func Load(_ context.Context, vfs avfs.VFS, dir string) (*Manifest, error) {
	path := filepath.Join(dir, "agentpack.yaml")

	data, err := vfs.ReadFile(path)
	if err != nil {
		if avfs.IsNotExist(err) {
			return nil, fmt.Errorf("agentpack.yaml not found in %s", dir)
		}
		return nil, fmt.Errorf("reading agentpack.yaml: %w", err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing agentpack.yaml: %w", err)
	}

	if err := validate(&m); err != nil {
		return nil, err
	}

	return &m, nil
}

// validate enforces the rules described in the package documentation.
// It relies on yaml.v3's behavior of setting a non-nil empty slice for an
// explicit "plugins: []" in YAML, versus nil for a missing key.
func validate(m *Manifest) error {
	// Detect conflicting forms: top-level name AND plugins array both present.
	if m.Name != "" && m.Plugins != nil {
		return fmt.Errorf(
			"manifest has both top-level 'name' and 'plugins'; use one or the other",
		)
	}

	// Explicit empty plugins list: plugins: [] was written but has no entries.
	if m.Plugins != nil && len(m.Plugins) == 0 {
		return fmt.Errorf("no plugins defined in agentpack.yaml")
	}

	// Multi-plugin form.
	if len(m.Plugins) > 0 {
		for i, p := range m.Plugins {
			if p.Name == "" {
				return fmt.Errorf("plugin[%d]: name is required", i)
			}
			if p.Version == "" {
				return fmt.Errorf("plugin[%d] %q: version is required", i, p.Name)
			}
			if p.Description == "" {
				return fmt.Errorf("plugin[%d] %q: description is required", i, p.Name)
			}
		}
		return nil
	}

	// Single-plugin form.
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("version is required")
	}
	if m.Description == "" {
		return fmt.Errorf("description is required")
	}

	return nil
}

// Normalize converts a single-plugin manifest into a one-element []Plugin
// slice, or returns the plugins slice for a multi-plugin manifest. In the
// multi-plugin case, shared fields (author, license, homepage) are inherited
// from the top-level manifest when the individual plugin does not set them.
func Normalize(m *Manifest) []Plugin {
	if len(m.Plugins) == 0 {
		// Single-plugin form: promote top-level fields into a Plugin.
		return []Plugin{
			{
				Name:        m.Name,
				Version:     m.Version,
				Description: m.Description,
				Author:      m.Author,
				License:     m.License,
				Homepage:    m.Homepage,
				Keywords:    m.Keywords,
				Category:    m.Category,
				Skills:      m.Skills,
				Commands:    m.Commands,
				Hooks:       m.Hooks,
				Agents:      m.Agents,
				MCP:         m.MCP,
				Binaries:    m.Binaries,
				Settings:    m.Settings,
			},
		}
	}

	// Multi-plugin form: apply shared field inheritance.
	plugins := make([]Plugin, len(m.Plugins))
	for i, p := range m.Plugins {
		if p.Author == (Author{}) {
			p.Author = m.Author
		}
		if p.License == "" {
			p.License = m.License
		}
		if p.Homepage == "" {
			p.Homepage = m.Homepage
		}
		plugins[i] = p
	}
	return plugins
}
