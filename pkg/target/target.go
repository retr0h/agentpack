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

// Package target defines the Target interface and shared types for
// agent-specific install drivers.
package target

import (
	"context"

	"github.com/retr0h/agentpack/pkg/metadata"
)

// Target knows how to install and list agentpack plugins for a specific AI
// coding agent.
type Target interface {
	// Name returns the agent identifier (e.g. "claude-code", "cursor").
	Name() string

	// DisplayName returns the human-readable agent name (e.g. "Claude Code").
	DisplayName() string

	// Detect returns true if this agent is installed on the current system.
	Detect() bool

	// Install places the unpacked archive content into the correct locations
	// for this agent.
	Install(ctx context.Context, opts InstallOpts) error

	// List returns all agentpack-managed plugins installed for this agent.
	List() ([]InstalledPlugin, error)
}

// InstallOpts contains everything needed to install a plugin into a target.
type InstallOpts struct {
	// Name is the plugin name from metadata.
	Name string

	// Version is the plugin version from metadata.
	Version string

	// SourceDir is the temp directory containing the extracted archive content.
	SourceDir string

	// Dir is the root directory for installation (cwd or home).
	Dir string

	// Meta is the build metadata read from the extracted archive.
	Meta *metadata.Metadata
}

// InstalledPlugin represents a single installed plugin found by a target.
type InstalledPlugin struct {
	// Name is the plugin identifier.
	Name string

	// Version is the plugin version.
	Version string

	// SHA is the short git commit SHA (first 7 characters).
	SHA string

	// Installed is the build timestamp, trimmed to YYYY-MM-DD.
	Installed string

	// Dir is the absolute path to the installed plugin directory.
	Dir string

	// Target is the name of the agent this plugin was found in.
	Target string
}
