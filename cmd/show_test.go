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

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/pkg/inspect"
	"github.com/retr0h/agentpack/pkg/registry"
)

type fakeRegistryLoader struct {
	manifest *registry.PackageManifest
	err      error
}

func (f *fakeRegistryLoader) Load(_ string) (*registry.PackageManifest, error) {
	return f.manifest, f.err
}

type fakeInspector struct {
	result *inspect.Result
	err    error
}

func (f *fakeInspector) Run(_ context.Context, _ inspect.Options) (*inspect.Result, error) {
	return f.result, f.err
}

func TestInfoCmd(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		format         string
		registryLoader *fakeRegistryLoader
		inspector      *fakeInspector
		wantErr        string
		wantOutput     string
		checkJSON      func(t *testing.T, output []byte)
	}{
		{
			name: "info for installed package",
			args: []string{"info", "my-plugin"},
			registryLoader: &fakeRegistryLoader{
				manifest: &registry.PackageManifest{
					Name:      "my-plugin",
					Source:    "https://github.com/org/repo",
					SHA:       "abcdef1234567890abcdef1234567890abcdef12",
					Version:   "v1.0.0",
					Installed: "2026-05-20T10:00:00Z",
					Scope:     registry.ScopeLocal,
					Files: []registry.InstalledFile{
						{
							Path:   "skills/intro.md",
							SHA256: "aabbccdd",
							Target: "claude-code",
							Dir:    "/home/user/.claude/plugins/my-plugin",
						},
					},
				},
			},
			wantOutput: "my-plugin",
		},
		{
			name: "info for archive file",
			args: []string{"info", "plugin-v1.0.0.agentpack"},
			inspector: &fakeInspector{
				result: &inspect.Result{
					Name:    "plugin",
					Version: "v1.0.0",
					Built:   "2026-05-20T10:00:00Z",
					SHA:     "deadbeef12345678",
					Files: []inspect.FileEntry{
						{
							Path:     "skills/intro.md",
							Size:     256,
							SHA256:   "aabbccdd",
							Verified: true,
						},
					},
					Total: 256,
				},
			},
			wantOutput: "plugin",
		},
		{
			name: "info for unknown package",
			args: []string{"info", "nonexistent"},
			registryLoader: &fakeRegistryLoader{
				err: fmt.Errorf("package %q not found in registry", "nonexistent"),
			},
			wantErr: "not found in registry",
		},
		{
			name:   "info with json output for installed package",
			args:   []string{"info", "json-plugin"},
			format: "json",
			registryLoader: &fakeRegistryLoader{
				manifest: &registry.PackageManifest{
					Name:      "json-plugin",
					Source:    "https://github.com/org/repo",
					SHA:       "abcdef1234567890abcdef1234567890abcdef12",
					Version:   "v2.0.0",
					Installed: "2026-05-20T10:00:00Z",
					Scope:     registry.ScopeLocal,
					Files: []registry.InstalledFile{
						{
							Path:   "skills/intro.md",
							SHA256: "aabbccdd",
							Target: "claude-code",
							Dir:    "/home/user/.claude/plugins/json-plugin",
						},
					},
				},
			},
			checkJSON: func(t *testing.T, output []byte) {
				t.Helper()
				var m registry.PackageManifest
				require.NoError(t, json.Unmarshal(output, &m))
				assert.Equal(t, "json-plugin", m.Name)
				assert.Equal(t, "v2.0.0", m.Version)
			},
		},
		{
			name:   "info with json output for archive file",
			args:   []string{"info", "archive.agentpack"},
			format: "json",
			inspector: &fakeInspector{
				result: &inspect.Result{
					Name:    "archive-plugin",
					Version: "v3.0.0",
					Built:   "2026-05-20T10:00:00Z",
					SHA:     "deadbeef",
					Files: []inspect.FileEntry{
						{
							Path:     "skills/intro.md",
							Size:     128,
							SHA256:   "eeff0011",
							Verified: true,
						},
					},
					Total: 128,
				},
			},
			checkJSON: func(t *testing.T, output []byte) {
				t.Helper()
				var r inspect.Result
				require.NoError(t, json.Unmarshal(output, &r))
				assert.Equal(t, "archive-plugin", r.Name)
				assert.Equal(t, "v3.0.0", r.Version)
			},
		},
		{
			name: "info archive error",
			args: []string{"info", "bad.agentpack"},
			inspector: &fakeInspector{
				err: fmt.Errorf("extract: file not found"),
			},
			wantErr: "extract: file not found",
		},
		{
			name: "info for installed package with empty scope defaults to local",
			args: []string{"info", "scoped-plugin"},
			registryLoader: &fakeRegistryLoader{
				manifest: &registry.PackageManifest{
					Name:      "scoped-plugin",
					Source:    "https://github.com/org/repo",
					SHA:       "abcdef1234567890abcdef1234567890abcdef12",
					Version:   "v1.0.0",
					Installed: "2026-05-20T10:00:00Z",
					Files: []registry.InstalledFile{
						{
							Path:   "skills/intro.md",
							SHA256: "aabbccdd",
							Target: "claude-code",
							Dir:    "/tmp/test",
						},
					},
				},
			},
			wantOutput: "local",
		},
		{
			name:    "info missing argument",
			args:    []string{"info"},
			wantErr: "accepts 1 arg(s)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origLoader := pkgRegistryLoader
			origInspector := pkgInspector
			origFormat := outputFormat
			t.Cleanup(func() {
				pkgRegistryLoader = origLoader
				pkgInspector = origInspector
				outputFormat = origFormat
			})

			if tt.registryLoader != nil {
				pkgRegistryLoader = tt.registryLoader
			}
			if tt.inspector != nil {
				pkgInspector = tt.inspector
			}
			if tt.format != "" {
				outputFormat = tt.format
			} else {
				outputFormat = "text"
			}

			buf := new(bytes.Buffer)
			rootCmd.SetOut(buf)
			rootCmd.SetErr(buf)
			rootCmd.SetArgs(tt.args)

			err := rootCmd.ExecuteContext(context.Background())

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.checkJSON != nil {
				tt.checkJSON(t, buf.Bytes())
				return
			}

			assert.Contains(t, buf.String(), tt.wantOutput)
		})
	}
}
