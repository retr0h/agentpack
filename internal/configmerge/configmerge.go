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

// Package configmerge handles merging MCP servers, hooks, and settings
// fragments into a JSON settings file (e.g. .claude/settings.json).
// All operations are read-modify-write; the file is created when absent.
package configmerge

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
)

// MergeResult records what was merged for reversibility.
type MergeResult struct {
	MCPServers []string // names of MCP servers added
	HookKeys   []string // hook event keys that had entries appended
}

// MergeMCP reads settingsPath, adds the MCP server under mcpServers key.
// Returns an error if a server with the same name already exists.
func MergeMCP(settingsPath string, name string, config map[string]any) error {
	doc, err := readSettings(settingsPath)
	if err != nil {
		return err
	}

	servers := getOrCreateMap(doc, "mcpServers")
	if _, exists := servers[name]; exists {
		return fmt.Errorf("MCP server %q already exists in %s", name, settingsPath)
	}

	servers[name] = config
	doc["mcpServers"] = servers

	return writeSettings(settingsPath, doc)
}

// MergeHooks reads settingsPath, appends hook entries tagged with pluginName.
// Each entry in hooks is a map of event name -> []entry. A "_plugin" field
// is injected into every hook entry so RemoveHooks can identify them later.
func MergeHooks(settingsPath string, pluginName string, hooks map[string]any) error {
	doc, err := readSettings(settingsPath)
	if err != nil {
		return err
	}

	hooksSection := getOrCreateMap(doc, "hooks")

	for event, rawEntries := range hooks {
		entries, ok := rawEntries.([]any)
		if !ok {
			continue
		}

		existing := getOrCreateSlice(hooksSection, event)

		for _, rawEntry := range entries {
			entry, ok := rawEntry.(map[string]any)
			if !ok {
				continue
			}

			tagged := make(map[string]any, len(entry)+1)
			maps.Copy(tagged, entry)

			tagged["_plugin"] = pluginName
			existing = append(existing, tagged)
		}

		hooksSection[event] = existing
	}

	doc["hooks"] = hooksSection

	return writeSettings(settingsPath, doc)
}

// MergeSettings reads settingsPath, merges key-value pairs from fragment
// into the top level. Existing keys outside the fragment are not touched.
func MergeSettings(settingsPath string, fragment map[string]any) error {
	doc, err := readSettings(settingsPath)
	if err != nil {
		return err
	}

	maps.Copy(doc, fragment)

	return writeSettings(settingsPath, doc)
}

// RemoveMCP removes a named MCP server from settingsPath.
// It is a no-op when the server does not exist.
func RemoveMCP(settingsPath string, name string) error {
	doc, err := readSettings(settingsPath)
	if err != nil {
		return err
	}

	servers, ok := doc["mcpServers"].(map[string]any)
	if !ok {
		return nil
	}

	delete(servers, name)
	doc["mcpServers"] = servers

	return writeSettings(settingsPath, doc)
}

// RemoveHooks removes all hook entries tagged with pluginName from settingsPath.
// Entries without a "_plugin" field are left untouched.
func RemoveHooks(settingsPath string, pluginName string) error {
	doc, err := readSettings(settingsPath)
	if err != nil {
		return err
	}

	hooksSection, ok := doc["hooks"].(map[string]any)
	if !ok {
		return nil
	}

	for event, rawEntries := range hooksSection {
		entries, ok := rawEntries.([]any)
		if !ok {
			continue
		}

		kept := entries[:0:0]

		for _, rawEntry := range entries {
			entry, ok := rawEntry.(map[string]any)
			if !ok {
				kept = append(kept, rawEntry)
				continue
			}

			if entry["_plugin"] == pluginName {
				continue
			}

			kept = append(kept, entry)
		}

		hooksSection[event] = kept
	}

	doc["hooks"] = hooksSection

	return writeSettings(settingsPath, doc)
}

// readSettings reads and unmarshals the JSON file at path into a map.
// When the file does not exist it returns an empty map rather than an error.
func readSettings(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, nil
		}

		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if doc == nil {
		doc = map[string]any{}
	}

	return doc, nil
}

// writeSettings marshals doc and writes it to path with 2-space indentation
// to match Claude Code's settings.json style. Parent directories are created
// as needed.
func writeSettings(path string, doc map[string]any) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

// getOrCreateMap returns the value at key as map[string]any, creating it
// when absent or when the existing value has an incompatible type.
func getOrCreateMap(doc map[string]any, key string) map[string]any {
	if v, ok := doc[key].(map[string]any); ok {
		return v
	}

	return map[string]any{}
}

// getOrCreateSlice returns the value at key in m as []any, creating it
// when absent or when the existing value has an incompatible type.
func getOrCreateSlice(m map[string]any, key string) []any {
	if v, ok := m[key].([]any); ok {
		return v
	}

	return []any{}
}
