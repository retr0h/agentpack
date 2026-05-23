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

package list_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/retr0h/claudia/pkg/list"
	"github.com/retr0h/claudia/pkg/metadata"
)

// writeMeta writes a metadata.json into pluginDir/marketplaces/<name>/.claudia/.
func writeMeta(t *testing.T, pluginDir string, meta metadata.Metadata) {
	t.Helper()

	dir := filepath.Join(pluginDir, "marketplaces", meta.Name, ".claudia")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0o644); err != nil {
		t.Fatalf("write metadata.json: %v", err)
	}
}

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T) string // returns pluginDir
		wantErr     string
		checkResult func(t *testing.T, entries []list.Entry)
	}{
		{
			name: "returns empty slice for empty plugin dir",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				if err := os.MkdirAll(filepath.Join(dir, "marketplaces"), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				return dir
			},
			checkResult: func(t *testing.T, entries []list.Entry) {
				t.Helper()
				if len(entries) != 0 {
					t.Errorf("entry count = %d, want 0", len(entries))
				}
			},
		},
		{
			name: "returns empty slice when marketplaces dir does not exist",
			setup: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			checkResult: func(t *testing.T, entries []list.Entry) {
				t.Helper()
				if len(entries) != 0 {
					t.Errorf("entry count = %d, want 0", len(entries))
				}
			},
		},
		{
			name: "returns one entry for a single installed plugin",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				writeMeta(t, dir, metadata.Metadata{
					Name:           "acme-toolkit",
					Version:        "1.0.0",
					GitCommitSHA:   "a1b2c3d4e5f6789",
					BuildTimestamp: "2026-05-23T10:00:00Z",
				})
				return dir
			},
			checkResult: func(t *testing.T, entries []list.Entry) {
				t.Helper()
				if len(entries) != 1 {
					t.Fatalf("entry count = %d, want 1", len(entries))
				}
				e := entries[0]
				if e.Name != "acme-toolkit" {
					t.Errorf("Name = %q, want %q", e.Name, "acme-toolkit")
				}
				if e.Version != "1.0.0" {
					t.Errorf("Version = %q, want %q", e.Version, "1.0.0")
				}
				if e.SHA != "a1b2c3d" {
					t.Errorf("SHA = %q, want %q", e.SHA, "a1b2c3d")
				}
				if e.Installed != "2026-05-23" {
					t.Errorf("Installed = %q, want %q", e.Installed, "2026-05-23")
				}
				if e.Dir == "" {
					t.Error("Dir is empty")
				}
			},
		},
		{
			name: "returns multiple entries sorted by name",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				writeMeta(t, dir, metadata.Metadata{
					Name:           "k8s-helpers",
					Version:        "2.0.0",
					GitCommitSHA:   "d4e5f6a7b8c9012",
					BuildTimestamp: "2026-05-23T09:00:00Z",
				})
				writeMeta(t, dir, metadata.Metadata{
					Name:           "acme-toolkit",
					Version:        "1.0.0",
					GitCommitSHA:   "a1b2c3d4e5f6789",
					BuildTimestamp: "2026-05-23T10:00:00Z",
				})
				return dir
			},
			checkResult: func(t *testing.T, entries []list.Entry) {
				t.Helper()
				if len(entries) != 2 {
					t.Fatalf("entry count = %d, want 2", len(entries))
				}
				if entries[0].Name != "acme-toolkit" {
					t.Errorf("entries[0].Name = %q, want %q", entries[0].Name, "acme-toolkit")
				}
				if entries[1].Name != "k8s-helpers" {
					t.Errorf("entries[1].Name = %q, want %q", entries[1].Name, "k8s-helpers")
				}
			},
		},
		{
			name: "skips non-claudia directories without metadata.json",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				// A directory without .claudia/metadata.json.
				nonClaudia := filepath.Join(dir, "marketplaces", "git-plugin")
				if err := os.MkdirAll(nonClaudia, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				// A proper claudia plugin.
				writeMeta(t, dir, metadata.Metadata{
					Name:           "acme-toolkit",
					Version:        "1.0.0",
					GitCommitSHA:   "a1b2c3d4e5f6789",
					BuildTimestamp: "2026-05-22T08:00:00Z",
				})
				return dir
			},
			checkResult: func(t *testing.T, entries []list.Entry) {
				t.Helper()
				if len(entries) != 1 {
					t.Fatalf("entry count = %d, want 1", len(entries))
				}
				if entries[0].Name != "acme-toolkit" {
					t.Errorf("Name = %q, want %q", entries[0].Name, "acme-toolkit")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pluginDir := tt.setup(t)
			entries, err := list.Run(pluginDir)

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

			if tt.checkResult != nil {
				tt.checkResult(t, entries)
			}
		})
	}
}
