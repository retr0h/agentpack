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
//
// Usage:
//
//	// Retrieve all auto-detected targets for the current machine.
//	targets := target.Detected()
//
//	// Install into a specific target.
//	err := target.Install(ctx, target.InstallOpts{
//	    Name:      "my-plugin",
//	    Version:   "v1.0.0",
//	    SourceDir: "/tmp/unpacked",
//	    Dir:       os.Getwd(),
//	})
//
// Target drivers are registered via init() using Register. The claudecode,
// cursor, and universal packages register their drivers automatically when
// imported with a blank import ("_") in cmd/root.go.
package target

import (
	"context"

	"github.com/retr0h/agentpack/internal/metadata"
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

	// SupportedTypes returns the content type identifiers this target can
	// install (e.g. "skill", "command", "hook", "agent", "mcp", "config").
	SupportedTypes() []string

	// Install places the unpacked archive content into the correct locations
	// for this agent. Returns the list of files written.
	Install(ctx context.Context, opts InstallOpts) ([]InstalledFile, error)

	// List returns all agentpack-managed plugins installed for this agent.
	List() ([]InstalledPlugin, error)
}

// ContentEntry describes a single content item declared in a plugin manifest.
type ContentEntry struct {
	// Name is the entry identifier (e.g. the skill or command name).
	Name string

	// Type is the content type (e.g. "skill", "command", "hook", "agent",
	// "mcp", "config").
	Type string

	// Root is the path of the entry's source directory relative to the
	// unpacked archive root.
	Root string
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

	// Global installs into the agent's global skills directory under the user's
	// home directory instead of the project-local directory.
	Global bool

	// Meta is the build metadata read from the extracted archive.
	Meta *metadata.Metadata

	// Entries is the list of content entries from the plugin manifest. The
	// install pipeline filters this list against SupportedTypes before passing
	// it to the driver.
	Entries []ContentEntry
}

// InstalledFile represents a single file written by a target's Install method.
type InstalledFile struct {
	// Path is the file path relative to the install base directory.
	Path string

	// SHA256 is the hex-encoded SHA-256 digest of the file content.
	SHA256 string
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
