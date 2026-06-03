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

// Package initpkg scaffolds a new agentpack skill project on disk.
//
// Usage:
//
//	s := initpkg.New()
//	err := s.Run(initpkg.Options{
//	    Name: "my-skill",
//	    Dir:  "/path/to/parent",
//	})
//
// Run creates the directory tree, writes SKILL.md and agentpack.yaml from
// their templates, and returns an error when agentpack.yaml already exists.
package initpkg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const skillMDTemplate = `---
name: %s
description: ""
---

# %s

Your skill instructions here.
`

const agentpackYAMLTemplate = `name: %s
version: 0.1.0
skills:
  - skills/**/*
`

// Options configures a scaffold run.
type Options struct {
	// Name is the skill name used in templates and directory layout.
	Name string

	// Dir is the base directory to scaffold into.
	Dir string
}

// Scaffold creates a new skill project skeleton.
type Scaffold struct{}

// New returns a new Scaffold.
func New() *Scaffold { return &Scaffold{} }

// Run creates the skill project skeleton under opts.Dir.
//
// It creates opts.Dir when it does not already exist, errors when
// agentpack.yaml already exists there, then writes:
//
//	skills/{name}/SKILL.md
//	agentpack.yaml
func (s *Scaffold) Run(opts Options) error {
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return fmt.Errorf("create dir %s: %w", opts.Dir, err)
	}

	yamlPath := filepath.Join(opts.Dir, "agentpack.yaml")
	if _, err := os.Stat(yamlPath); err == nil {
		return errors.New("agentpack.yaml already exists")
	}

	skillDir := filepath.Join(opts.Dir, "skills", opts.Name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}

	skillMD := filepath.Join(skillDir, "SKILL.md")
	skillContent := fmt.Sprintf(skillMDTemplate, opts.Name, opts.Name)
	if err := os.WriteFile(skillMD, []byte(skillContent), 0o644); err != nil {
		return fmt.Errorf("write SKILL.md: %w", err)
	}

	yamlContent := fmt.Sprintf(agentpackYAMLTemplate, opts.Name)
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0o644); err != nil {
		return fmt.Errorf("write agentpack.yaml: %w", err)
	}

	return nil
}
