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

package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/retr0h/agentpack/internal/configmerge"
	"github.com/retr0h/agentpack/internal/target"
)

// mcpNameRe constrains MCP server names to a safe identifier so a
// package-controlled "name" cannot inject path separators or overwrite
// unrelated entries when written as a JSON key into settings.
var mcpNameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// ValidateMCPName rejects a package-controlled MCP server name that is empty
// or contains anything outside letters, digits, '.', '_', and '-'. Every
// driver that writes an MCP "name" as a config key (whether it delegates to
// InstallMCP or merges into its own format) must call this first so a crafted
// archive cannot inject separators or clobber unrelated entries.
func ValidateMCPName(name string) error {
	if name == "" {
		return fmt.Errorf("mcp server name is empty")
	}

	if !mcpNameRe.MatchString(name) {
		return fmt.Errorf(
			"invalid mcp server name %q (allowed: letters, digits, '.', '_', '-')",
			name,
		)
	}

	return nil
}

// InstallSkillEntry copies a single skill/command/agent entry into the
// target directory and returns the list of files written.
func InstallSkillEntry(
	ctx context.Context,
	entry target.ContentEntry,
	skillsDir string,
	baseDir string,
	mkdirAll func(string, os.FileMode) error,
) ([]target.InstalledFile, error) {
	destDir := filepath.Join(skillsDir, entry.Name)

	if err := mkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir skills dir: %w", err)
	}

	if err := CopyTreeIfExists(ctx, entry.Root, destDir); err != nil {
		return nil, fmt.Errorf("copy skills: %w", err)
	}

	return EnumerateFiles(ctx, destDir, baseDir)
}

// InstallMCP merges all mcp/*.json files from srcDir into mcpPath.
// Each JSON file must contain a "name" field identifying the MCP server;
// the remaining fields are passed to configmerge.MergeMCP.
// If srcDir/mcp/ does not exist the call is a no-op.
func InstallMCP(_ context.Context, srcDir, mcpPath string) error {
	mcpDir := filepath.Join(srcDir, "mcp")
	if _, err := os.Stat(mcpDir); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	entries, err := os.ReadDir(mcpDir)
	if err != nil {
		return fmt.Errorf("read mcp dir: %w", err)
	}

	for _, de := range entries {
		if de.IsDir() || filepath.Ext(de.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(mcpDir, de.Name()))
		if err != nil {
			return fmt.Errorf("read mcp/%s: %w", de.Name(), err)
		}

		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("parse mcp/%s: %w", de.Name(), err)
		}

		name, _ := raw["name"].(string)
		if err := ValidateMCPName(name); err != nil {
			return fmt.Errorf("mcp/%s: %w", de.Name(), err)
		}

		delete(raw, "name")

		if err := configmerge.MergeMCP(mcpPath, name, raw); err != nil {
			return fmt.Errorf("merge mcp %q: %w", name, err)
		}
	}

	return nil
}

// InstallHooksJSON merges hooks/hooks.json from srcDir into hooksPath using
// configmerge.MergeHooks. If srcDir/hooks/hooks.json does not exist the call
// is a no-op.
func InstallHooksJSON(_ context.Context, srcDir, hooksPath, pluginName string) error {
	hooksFile := filepath.Join(srcDir, "hooks", "hooks.json")
	if _, err := os.Stat(hooksFile); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	data, err := os.ReadFile(hooksFile)
	if err != nil {
		return fmt.Errorf("read hooks/hooks.json: %w", err)
	}

	var hooks map[string]any
	if err := json.Unmarshal(data, &hooks); err != nil {
		return fmt.Errorf("parse hooks/hooks.json: %w", err)
	}

	if err := configmerge.MergeHooks(hooksPath, pluginName, hooks); err != nil {
		return fmt.Errorf("merge hooks: %w", err)
	}

	return nil
}
