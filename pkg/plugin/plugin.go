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

// Package plugin generates Claude Code plugin directory structures.
package plugin

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/retr0h/claudia/pkg/manifest"
)

// Marketplace is the Claude Code marketplace descriptor written to
// .claude-plugin/marketplace.json.
type Marketplace struct {
	Name     string              `json:"name"`
	Owner    MarketplaceOwner    `json:"owner"`
	Metadata MarketplaceMetadata `json:"metadata"`
	Plugins  []MarketplacePlugin `json:"plugins"`
}

// MarketplaceOwner identifies the marketplace maintainer.
type MarketplaceOwner struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// MarketplaceMetadata holds the marketplace-level description and version.
type MarketplaceMetadata struct {
	Description string `json:"description"`
	Version     string `json:"version"`
}

// MarketplacePlugin is a single plugin entry inside a marketplace descriptor.
type MarketplacePlugin struct {
	Name        string   `json:"name"`
	Source      string   `json:"source"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Author      *Author  `json:"author,omitempty"`
	License     string   `json:"license,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
	Category    string   `json:"category,omitempty"`
}

// Descriptor is the Claude Code plugin descriptor written to
// .claude-plugin/plugin.json.
type Descriptor struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Author      *Author  `json:"author,omitempty"`
	License     string   `json:"license,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
	Commands    []string `json:"commands,omitempty"`
}

// Author holds attribution for a plugin.
type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// MCPConfig is the .mcp.json structure Claude Code reads to discover MCP
// servers.
type MCPConfig struct {
	MCPServers map[string]MCPServer `json:"mcpServers"`
}

// MCPServer is a single server entry inside an MCPConfig.
type MCPServer struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	URL     string            `json:"url,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// GenerateMarketplace produces the JSON bytes for marketplace.json from a
// manifest plugin. It uses the single-plugin marketplace pattern where the
// marketplace IS the plugin.
func GenerateMarketplace(p manifest.Plugin) ([]byte, error) {
	mp := MarketplacePlugin{
		Name:        p.Name,
		Source:      "./",
		Description: p.Description,
		Version:     p.Version,
		License:     p.License,
		Keywords:    p.Keywords,
		Category:    p.Category,
	}

	if p.Author != (manifest.Author{}) {
		mp.Author = &Author{
			Name:  p.Author.Name,
			Email: p.Author.Email,
		}
	}

	m := Marketplace{
		Name: p.Name,
		Owner: MarketplaceOwner{
			Name:  p.Author.Name,
			Email: p.Author.Email,
		},
		Metadata: MarketplaceMetadata{
			Description: p.Description,
			Version:     p.Version,
		},
		Plugins: []MarketplacePlugin{mp},
	}

	return json.MarshalIndent(m, "", "  ")
}

// GeneratePlugin produces the JSON bytes for plugin.json. commandPaths are
// the resolved destination paths for command files (e.g. "commands/init.md");
// each is prefixed with "./" in the output.
func GeneratePlugin(p manifest.Plugin, commandPaths []string) ([]byte, error) {
	pd := Descriptor{
		Name:        p.Name,
		Version:     p.Version,
		Description: p.Description,
		License:     p.License,
		Keywords:    p.Keywords,
	}

	if p.Author != (manifest.Author{}) {
		pd.Author = &Author{
			Name:  p.Author.Name,
			Email: p.Author.Email,
		}
	}

	if len(commandPaths) > 0 {
		cmds := make([]string, len(commandPaths))
		for i, c := range commandPaths {
			cmds[i] = "./" + c
		}
		pd.Commands = cmds
	}

	return json.MarshalIndent(pd, "", "  ")
}

// GenerateMCPConfig produces the JSON bytes for .mcp.json from MCP entries.
// Entries with a Config field set are skipped (they are included as-is by the
// build pipeline). Returns nil, nil when no entries need generation.
func GenerateMCPConfig(entries []manifest.MCPEntry) ([]byte, error) {
	cfg := MCPConfig{
		MCPServers: make(map[string]MCPServer),
	}

	for _, e := range entries {
		if e.Config != "" {
			continue
		}

		switch e.Type {
		case "binary":
			if e.Name == "" {
				return nil, fmt.Errorf("binary mcp entry requires a name")
			}
			if e.Src == "" {
				return nil, fmt.Errorf("binary mcp entry %q requires src", e.Name)
			}
			server := MCPServer{
				Command: "${CLAUDE_PLUGIN_ROOT}/mcp/" + filepath.Base(e.Src),
			}
			if len(e.Args) > 0 {
				server.Args = e.Args
			}
			if len(e.Env) > 0 {
				server.Env = e.Env
			}
			cfg.MCPServers[e.Name] = server

		case "remote":
			if e.Name == "" {
				return nil, fmt.Errorf("remote mcp entry requires a name")
			}
			if e.URL == "" {
				return nil, fmt.Errorf("remote mcp entry %q requires url", e.Name)
			}
			cfg.MCPServers[e.Name] = MCPServer{
				URL: e.URL,
			}

		case "ux":
			if e.Name == "" {
				return nil, fmt.Errorf("ux mcp entry requires a name")
			}
			if e.Package == "" {
				return nil, fmt.Errorf("ux mcp entry %q requires package", e.Name)
			}
			args := []string{e.Package}
			if len(e.Args) > 0 {
				args = append(args, e.Args...)
			}
			cfg.MCPServers[e.Name] = MCPServer{
				Command: "npx",
				Args:    args,
			}

		default:
			return nil, fmt.Errorf(
				"unknown mcp type %q; expected binary, remote, or ux",
				e.Type,
			)
		}
	}

	if len(cfg.MCPServers) == 0 {
		return nil, nil
	}

	return json.MarshalIndent(cfg, "", "  ")
}
