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

package plugin_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/retr0h/agentpack/internal/plugin"
	"github.com/retr0h/agentpack/pkg/manifest"
)

func TestGenerateMarketplace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		plugin    manifest.Plugin
		checkJSON func(t *testing.T, data map[string]any)
	}{
		{
			name: "full plugin with all fields",
			plugin: manifest.Plugin{
				Name:        "zeek-pros",
				Version:     "1.0.0",
				Description: "Zeek protocol analysis skills",
				Author:      manifest.Author{Name: "John Dewey", Email: "john@dewey.ws"},
				License:     "MIT",
				Keywords:    []string{"zeek", "security"},
				Category:    "security",
			},
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				if data["name"] != "zeek-pros" {
					t.Errorf("name = %v, want zeek-pros", data["name"])
				}
				owner := data["owner"].(map[string]any)
				if owner["name"] != "John Dewey" {
					t.Errorf("owner.name = %v, want John Dewey", owner["name"])
				}
				meta := data["metadata"].(map[string]any)
				if meta["version"] != "1.0.0" {
					t.Errorf("metadata.version = %v, want 1.0.0", meta["version"])
				}
				plugins := data["plugins"].([]any)
				if len(plugins) != 1 {
					t.Fatalf("plugins length = %d, want 1", len(plugins))
				}
				p := plugins[0].(map[string]any)
				if p["source"] != "./" {
					t.Errorf("plugins[0].source = %v, want ./", p["source"])
				}
				if p["category"] != "security" {
					t.Errorf("plugins[0].category = %v, want security", p["category"])
				}
				author := p["author"].(map[string]any)
				if author["email"] != "john@dewey.ws" {
					t.Errorf("plugins[0].author.email = %v, want john@dewey.ws", author["email"])
				}
			},
		},
		{
			name: "minimal plugin without author",
			plugin: manifest.Plugin{
				Name:        "my-plugin",
				Version:     "0.1.0",
				Description: "A plugin",
			},
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				plugins := data["plugins"].([]any)
				p := plugins[0].(map[string]any)
				if _, ok := p["author"]; ok {
					t.Error("expected no author field for empty author")
				}
				if p["source"] != "./" {
					t.Errorf("source = %v, want ./", p["source"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := plugin.GenerateMarketplace(tt.plugin)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var data map[string]any
			if err := json.Unmarshal(got, &data); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}

			tt.checkJSON(t, data)
		})
	}
}

func TestGeneratePlugin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		plugin       manifest.Plugin
		commandPaths []string
		checkJSON    func(t *testing.T, data map[string]any)
	}{
		{
			name: "plugin with commands",
			plugin: manifest.Plugin{
				Name:        "git-commands",
				Version:     "2.1.0",
				Description: "Git workflow commands",
				Author:      manifest.Author{Name: "John Dewey", Email: "john@dewey.ws"},
				License:     "MIT",
				Keywords:    []string{"git"},
			},
			commandPaths: []string{"commands/init.md", "commands/push.md"},
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				if data["name"] != "git-commands" {
					t.Errorf("name = %v, want git-commands", data["name"])
				}
				cmds := data["commands"].([]any)
				if len(cmds) != 2 {
					t.Fatalf("commands length = %d, want 2", len(cmds))
				}
				if cmds[0] != "./commands/init.md" {
					t.Errorf("commands[0] = %v, want ./commands/init.md", cmds[0])
				}
				if cmds[1] != "./commands/push.md" {
					t.Errorf("commands[1] = %v, want ./commands/push.md", cmds[1])
				}
			},
		},
		{
			name: "plugin without commands",
			plugin: manifest.Plugin{
				Name:        "my-skills",
				Version:     "1.0.0",
				Description: "Skills only",
			},
			commandPaths: nil,
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				if _, ok := data["commands"]; ok {
					t.Error("expected no commands field when none provided")
				}
			},
		},
		{
			name: "plugin without author",
			plugin: manifest.Plugin{
				Name:        "minimal",
				Version:     "0.1.0",
				Description: "Minimal plugin",
			},
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				if _, ok := data["author"]; ok {
					t.Error("expected no author field for empty author")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := plugin.GeneratePlugin(tt.plugin, tt.commandPaths)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var data map[string]any
			if err := json.Unmarshal(got, &data); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}

			tt.checkJSON(t, data)
		})
	}
}

func TestGenerateMCPConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		entries   []manifest.MCPEntry
		wantNil   bool
		wantErr   string
		checkJSON func(t *testing.T, data map[string]any)
	}{
		{
			name: "remote mcp server",
			entries: []manifest.MCPEntry{
				{
					Type: "remote",
					Name: "my-remote",
					URL:  "https://mcp.example.com/v1",
				},
			},
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				servers := data["mcpServers"].(map[string]any)
				s := servers["my-remote"].(map[string]any)
				if s["url"] != "https://mcp.example.com/v1" {
					t.Errorf("url = %v", s["url"])
				}
				if _, ok := s["command"]; ok {
					t.Error("remote server should not have command")
				}
			},
		},
		{
			name: "ux mcp server",
			entries: []manifest.MCPEntry{
				{
					Type:    "ux",
					Name:    "my-ux",
					Package: "@mycompany/my-server",
					Args:    []string{"--verbose"},
				},
			},
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				servers := data["mcpServers"].(map[string]any)
				s := servers["my-ux"].(map[string]any)
				if s["command"] != "npx" {
					t.Errorf("command = %v, want npx", s["command"])
				}
				args := s["args"].([]any)
				if len(args) != 2 {
					t.Fatalf("args length = %d, want 2", len(args))
				}
				if args[0] != "@mycompany/my-server" {
					t.Errorf("args[0] = %v", args[0])
				}
				if args[1] != "--verbose" {
					t.Errorf("args[1] = %v", args[1])
				}
			},
		},
		{
			name: "config entries are skipped",
			entries: []manifest.MCPEntry{
				{
					Type:   "remote",
					Name:   "skip-me",
					Config: ".mcp.json",
				},
			},
			wantNil: true,
		},
		{
			name:    "empty entries returns nil",
			entries: []manifest.MCPEntry{},
			wantNil: true,
		},
		{
			name: "binary type returns error",
			entries: []manifest.MCPEntry{
				{Type: "binary", Name: "my-server"},
			},
			wantErr: "binary mcp type is not supported",
		},
		{
			name: "unknown type returns error",
			entries: []manifest.MCPEntry{
				{Type: "magic", Name: "bad"},
			},
			wantErr: "unknown mcp type",
		},
		{
			name: "remote without name returns error",
			entries: []manifest.MCPEntry{
				{Type: "remote", URL: "https://example.com"},
			},
			wantErr: "requires a name",
		},
		{
			name: "remote without url returns error",
			entries: []manifest.MCPEntry{
				{Type: "remote", Name: "my-remote"},
			},
			wantErr: "requires url",
		},
		{
			name: "ux without name returns error",
			entries: []manifest.MCPEntry{
				{Type: "ux", Package: "@mycompany/pkg"},
			},
			wantErr: "requires a name",
		},
		{
			name: "ux without package returns error",
			entries: []manifest.MCPEntry{
				{Type: "ux", Name: "my-ux"},
			},
			wantErr: "requires package",
		},
		{
			name: "multiple servers in one config",
			entries: []manifest.MCPEntry{
				{Type: "ux", Name: "ux-srv", Package: "@example/pkg"},
				{Type: "remote", Name: "rem-srv", URL: "https://example.com"},
			},
			checkJSON: func(t *testing.T, data map[string]any) {
				t.Helper()
				servers := data["mcpServers"].(map[string]any)
				if len(servers) != 2 {
					t.Errorf("server count = %d, want 2", len(servers))
				}
				if _, ok := servers["ux-srv"]; !ok {
					t.Error("missing ux-srv")
				}
				if _, ok := servers["rem-srv"]; !ok {
					t.Error("missing rem-srv")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := plugin.GenerateMCPConfig(tt.entries)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %s", got)
				}
				return
			}

			var data map[string]any
			if err := json.Unmarshal(got, &data); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}

			if tt.checkJSON != nil {
				tt.checkJSON(t, data)
			}
		})
	}
}
