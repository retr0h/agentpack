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

package safety_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/safety"
)

// --------------------------------------------------------------------------
// TestClassifyFile
// --------------------------------------------------------------------------

func TestClassifyFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		content []byte
		want    safety.Category
	}{
		{
			name:    "md extension is safe",
			path:    "skills/review/SKILL.md",
			content: []byte("# Review\n"),
			want:    safety.Safe,
		},
		{
			name:    "json extension is safe",
			path:    "mcp/server.json",
			content: []byte(`{"name":"server"}`),
			want:    safety.Safe,
		},
		{
			name:    "yaml extension is safe",
			path:    "config.yaml",
			content: []byte("key: value\n"),
			want:    safety.Safe,
		},
		{
			name:    "yml extension is safe",
			path:    "config.yml",
			content: []byte("key: value\n"),
			want:    safety.Safe,
		},
		{
			name:    "txt extension is safe",
			path:    "notes.txt",
			content: []byte("some notes"),
			want:    safety.Safe,
		},
		{
			name:    "toml extension is safe",
			path:    "config.toml",
			content: []byte("[section]\nkey = \"val\"\n"),
			want:    safety.Safe,
		},
		{
			name:    "sh extension is executable",
			path:    "hooks/lint.sh",
			content: []byte("echo hello\n"),
			want:    safety.Executable,
		},
		{
			name:    "py extension is executable",
			path:    "scripts/agent.py",
			content: []byte("print('hi')\n"),
			want:    safety.Executable,
		},
		{
			name:    "js extension is executable",
			path:    "scripts/run.js",
			content: []byte("console.log('hi');\n"),
			want:    safety.Executable,
		},
		{
			name:    "ts extension is executable",
			path:    "scripts/run.ts",
			content: []byte("const x = 1;\n"),
			want:    safety.Executable,
		},
		{
			name:    "rb extension is executable",
			path:    "scripts/run.rb",
			content: []byte("puts 'hi'\n"),
			want:    safety.Executable,
		},
		{
			name:    "pl extension is executable",
			path:    "scripts/run.pl",
			content: []byte("print 'hi';\n"),
			want:    safety.Executable,
		},
		{
			name:    "lua extension is executable",
			path:    "scripts/run.lua",
			content: []byte("print('hi')\n"),
			want:    safety.Executable,
		},
		{
			name:    "shebang makes any file executable regardless of extension",
			path:    "hooks/helper",
			content: []byte("#!/bin/bash\necho hi\n"),
			want:    safety.Executable,
		},
		{
			name:    "shebang on md file overrides safe extension",
			path:    "hooks/script.md",
			content: []byte("#!/usr/bin/env python3\n# a script\n"),
			want:    safety.Executable,
		},
		{
			name:    "ELF binary is detected",
			path:    "helpers/tool",
			content: []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01},
			want:    safety.Binary,
		},
		{
			name:    "Mach-O 32-bit is detected",
			path:    "helpers/tool",
			content: []byte{0xfe, 0xed, 0xfa, 0xce, 0x00},
			want:    safety.Binary,
		},
		{
			name:    "Mach-O 64-bit is detected",
			path:    "helpers/tool",
			content: []byte{0xfe, 0xed, 0xfa, 0xcf, 0x00},
			want:    safety.Binary,
		},
		{
			name:    "Mach-O fat binary is detected",
			path:    "helpers/tool",
			content: []byte{0xca, 0xfe, 0xba, 0xbe, 0x00},
			want:    safety.Binary,
		},
		{
			name:    "PE binary is detected",
			path:    "helpers/tool.exe",
			content: []byte{'M', 'Z', 0x90, 0x00},
			want:    safety.Binary,
		},
		{
			name:    "unrecognized extension with no shebang or magic bytes is safe",
			path:    "hooks/config",
			content: []byte("some config data\n"),
			want:    safety.Safe,
		},
		{
			name:    "empty content is safe",
			path:    "empty.md",
			content: []byte{},
			want:    safety.Safe,
		},
		{
			name:    "single byte content is safe",
			path:    "tiny",
			content: []byte{0x42},
			want:    safety.Safe,
		},
		{
			name:    "extension matching is case-insensitive",
			path:    "hooks/SCRIPT.SH",
			content: []byte("echo hi\n"),
			want:    safety.Executable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := safety.ClassifyFile(tt.path, tt.content)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --------------------------------------------------------------------------
// TestClassify
// --------------------------------------------------------------------------

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		files       map[string][]byte
		wantErr     string
		checkResult func(t *testing.T, c *safety.Classification)
	}{
		{
			name: "all safe files returns classification with no executables",
			files: map[string][]byte{
				"skills/review/SKILL.md": []byte("# Review\n"),
				"mcp/server.json":        []byte(`{"name":"server"}`),
				"commands/scan.md":       []byte("# Scan\n"),
			},
			checkResult: func(t *testing.T, c *safety.Classification) {
				t.Helper()
				assert.Len(t, c.Safe, 3)
				assert.Empty(t, c.Executable)
			},
		},
		{
			name: "executable files appear in executable list",
			files: map[string][]byte{
				"hooks/lint.sh":    []byte("#!/bin/bash\necho hi\n"),
				"scripts/agent.py": []byte("print('hi')\n"),
			},
			checkResult: func(t *testing.T, c *safety.Classification) {
				t.Helper()
				assert.Empty(t, c.Safe)
				assert.Len(t, c.Executable, 2)
			},
		},
		{
			name: "mixed safe and executable files",
			files: map[string][]byte{
				"skills/review/SKILL.md": []byte("# Review\n"),
				"commands/scan.md":       []byte("# Scan\n"),
				"mcp/server.json":        []byte(`{}`),
				"hooks/lint.sh":          []byte("#!/bin/bash\necho hi\n"),
				"scripts/agent.py":       []byte("print('hi')\n"),
			},
			checkResult: func(t *testing.T, c *safety.Classification) {
				t.Helper()
				assert.Len(t, c.Safe, 3)
				assert.Len(t, c.Executable, 2)
			},
		},
		{
			name: "ELF binary returns error",
			files: map[string][]byte{
				"helpers/tool": {0x7f, 'E', 'L', 'F', 0x02, 0x01},
			},
			wantErr: "binary file detected: helpers/tool (ELF executable)",
		},
		{
			name: "PE binary returns error",
			files: map[string][]byte{
				"helpers/tool.exe": {'M', 'Z', 0x90, 0x00},
			},
			wantErr: "binary file detected: helpers/tool.exe (PE executable)",
		},
		{
			name: "Mach-O binary returns error",
			files: map[string][]byte{
				"helpers/mac-tool": {0xfe, 0xed, 0xfa, 0xce, 0x00},
			},
			wantErr: "binary file detected: helpers/mac-tool (Mach-O executable)",
		},
		{
			name:  "empty file map returns empty classification",
			files: map[string][]byte{},
			checkResult: func(t *testing.T, c *safety.Classification) {
				t.Helper()
				assert.Empty(t, c.Safe)
				assert.Empty(t, c.Executable)
			},
		},
		{
			name: "shebang file with no extension is executable",
			files: map[string][]byte{
				"hooks/helper": []byte("#!/usr/bin/env python3\nprint('hi')\n"),
			},
			checkResult: func(t *testing.T, c *safety.Classification) {
				t.Helper()
				assert.Empty(t, c.Safe)
				assert.Len(t, c.Executable, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, err := safety.Classify(tt.files)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.Nil(t, c)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, c)

			if tt.checkResult != nil {
				tt.checkResult(t, c)
			}
		})
	}
}
