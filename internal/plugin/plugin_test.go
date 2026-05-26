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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/manifest"
	"github.com/retr0h/agentpack/internal/plugin"
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
				assert.Equal(t, "zeek-pros", data["name"])
				owner := data["owner"].(map[string]any)
				assert.Equal(t, "John Dewey", owner["name"])
				meta := data["metadata"].(map[string]any)
				assert.Equal(t, "1.0.0", meta["version"])
				plugins := data["plugins"].([]any)
				require.Len(t, plugins, 1)
				p := plugins[0].(map[string]any)
				assert.Equal(t, "./", p["source"])
				assert.Equal(t, "security", p["category"])
				author := p["author"].(map[string]any)
				assert.Equal(t, "john@dewey.ws", author["email"])
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
				assert.NotContains(t, p, "author")
				assert.Equal(t, "./", p["source"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := plugin.GenerateMarketplace(tt.plugin)
			require.NoError(t, err)

			var data map[string]any
			err = json.Unmarshal(got, &data)
			require.NoError(t, err)

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
				assert.Equal(t, "git-commands", data["name"])
				cmds := data["commands"].([]any)
				require.Len(t, cmds, 2)
				assert.Equal(t, "./commands/init.md", cmds[0])
				assert.Equal(t, "./commands/push.md", cmds[1])
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
				assert.NotContains(t, data, "commands")
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
				assert.NotContains(t, data, "author")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := plugin.GeneratePlugin(tt.plugin, tt.commandPaths)
			require.NoError(t, err)

			var data map[string]any
			err = json.Unmarshal(got, &data)
			require.NoError(t, err)

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
				assert.Equal(t, "https://mcp.example.com/v1", s["url"])
				assert.NotContains(t, s, "command")
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
				assert.Equal(t, "npx", s["command"])
				args := s["args"].([]any)
				require.Len(t, args, 2)
				assert.Equal(t, "@mycompany/my-server", args[0])
				assert.Equal(t, "--verbose", args[1])
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
				assert.Len(t, servers, 2)
				assert.Contains(t, servers, "ux-srv")
				assert.Contains(t, servers, "rem-srv")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := plugin.GenerateMCPConfig(tt.entries)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.wantNil {
				assert.Nil(t, got)
				return
			}

			var data map[string]any
			err = json.Unmarshal(got, &data)
			require.NoError(t, err)

			if tt.checkJSON != nil {
				tt.checkJSON(t, data)
			}
		})
	}
}
