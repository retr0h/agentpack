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

package agents_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/pkg/target"
	"github.com/retr0h/agentpack/pkg/target/agents"
)

func TestAgents_AllRegistered(t *testing.T) {
	t.Parallel()

	wantNames := make([]string, len(agents.Registry))
	for i, def := range agents.Registry {
		wantNames[i] = def.Name
	}

	tests := make([]struct {
		name     string
		wantName string
	}, 0, len(wantNames))
	for _, n := range wantNames {
		tests = append(tests, struct {
			name     string
			wantName string
		}{name: n, wantName: n})
	}

	all := target.All()
	registeredNames := make(map[string]bool, len(all))
	for _, tgt := range all {
		registeredNames[tgt.Name()] = true
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.True(t, registeredNames[tt.wantName])
		})
	}
}

func TestAgent_NameAndDisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		def         agents.AgentDef
		wantName    string
		wantDisplay string
	}{
		{
			name:        "cursor",
			def:         agents.AgentDef{Name: "cursor", Display: "Cursor", DetectHome: ".cursor"},
			wantName:    "cursor",
			wantDisplay: "Cursor",
		},
		{
			name:        "copilot",
			def:         agents.AgentDef{Name: "copilot", Display: "GitHub Copilot", DetectHome: ".copilot"},
			wantName:    "copilot",
			wantDisplay: "GitHub Copilot",
		},
		{
			name:        "opencode",
			def:         agents.AgentDef{Name: "opencode", Display: "OpenCode", DetectConfig: "opencode"},
			wantName:    "opencode",
			wantDisplay: "OpenCode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := agents.NewAgentWithFuncs(tt.def, os.UserHomeDir, os.Getwd)
			assert.Equal(t, tt.wantName, a.Name())
			assert.Equal(t, tt.wantDisplay, a.DisplayName())
		})
	}
}

func TestAgent_Detect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		def          agents.AgentDef
		homeFunc     func(t *testing.T) func() (string, error)
		getenvFunc   func(t *testing.T) func(string) string
		wantDetected bool
	}{
		{
			name: "detects via DetectHome when dir exists",
			def:  agents.AgentDef{Name: "cursor", Display: "Cursor", DetectHome: ".cursor"},
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				home := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(home, ".cursor"), 0o755))
				return func() (string, error) { return home, nil }
			},
			wantDetected: true,
		},
		{
			name: "not detected via DetectHome when dir absent",
			def:  agents.AgentDef{Name: "cursor", Display: "Cursor", DetectHome: ".cursor"},
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				home := t.TempDir()
				return func() (string, error) { return home, nil }
			},
			wantDetected: false,
		},
		{
			name: "home error returns false",
			def:  agents.AgentDef{Name: "cursor", Display: "Cursor", DetectHome: ".cursor"},
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				return func() (string, error) { return "", errors.New("no home") }
			},
			wantDetected: false,
		},
		{
			name: "EnvOverride set to existing dir returns true",
			def:  agents.AgentDef{Name: "codex", Display: "Codex", DetectHome: ".codex", EnvOverride: "CODEX_HOME"},
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				return func() (string, error) { return t.TempDir(), nil }
			},
			getenvFunc: func(t *testing.T) func(string) string {
				t.Helper()
				dir := t.TempDir()
				return func(key string) string {
					if key == "CODEX_HOME" {
						return dir
					}
					return ""
				}
			},
			wantDetected: true,
		},
		{
			name: "EnvOverride set to missing dir returns false",
			def:  agents.AgentDef{Name: "codex", Display: "Codex", DetectHome: ".codex", EnvOverride: "CODEX_HOME"},
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				return func() (string, error) { return t.TempDir(), nil }
			},
			getenvFunc: func(t *testing.T) func(string) string {
				t.Helper()
				return func(key string) string {
					if key == "CODEX_HOME" {
						return "/nonexistent/path/for/agentpack/test"
					}
					return ""
				}
			},
			wantDetected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			homeFunc := tt.homeFunc(t)

			if tt.getenvFunc != nil {
				getenv := tt.getenvFunc(t)
				a := agents.NewAgentWithGetenv(tt.def, homeFunc, getenv)
				assert.Equal(t, tt.wantDetected, a.Detect())
				return
			}

			a := agents.NewAgentWithFuncs(tt.def, homeFunc, os.Getwd)
			assert.Equal(t, tt.wantDetected, a.Detect())
		})
	}
}

func TestAgent_Install(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		def       agents.AgentDef
		setupSrc  func(t *testing.T) string
		cancelCtx bool
		wantErr   string
		check     func(t *testing.T, destBase string, pluginName string)
	}{
		{
			name: "copies skills into .agents/skills/{name}/",
			def:  agents.AgentDef{Name: "cursor", Display: "Cursor", DetectHome: ".cursor"},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				src := t.TempDir()
				skillsDir := filepath.Join(src, "skills")
				require.NoError(t, os.MkdirAll(skillsDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(skillsDir, "my-skill.md"),
					[]byte("# My Skill"),
					0o644,
				))
				return src
			},
			check: func(t *testing.T, destBase string, pluginName string) {
				t.Helper()
				tgt := filepath.Join(destBase, ".agents", "skills", pluginName, "my-skill.md")
				_, err := os.Stat(tgt)
				assert.NoError(t, err)
			},
		},
		{
			name: "no skills dir is a no-op",
			def:  agents.AgentDef{Name: "gemini", Display: "Gemini CLI", DetectHome: ".gemini"},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
		},
		{
			name:      "cancelled context returns error",
			def:       agents.AgentDef{Name: "cursor", Display: "Cursor", DetectHome: ".cursor"},
			cancelCtx: true,
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: "context canceled",
		},
		{
			name: "cwdFunc error propagates",
			def:  agents.AgentDef{Name: "cursor", Display: "Cursor", DetectHome: ".cursor"},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: "getwd",
		},
		{
			name: "mkdirAll failure propagates error",
			def:  agents.AgentDef{Name: "cursor", Display: "Cursor", DetectHome: ".cursor"},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: "mkdir agents skills dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir := tt.setupSrc(t)
			destBase := t.TempDir()

			cwdFunc := func() (string, error) { return destBase, nil }
			if tt.wantErr == "getwd" {
				cwdFunc = func() (string, error) { return "", errors.New("getwd failed") }
			}
			if tt.wantErr == "mkdir agents skills dir" {
				roDir := t.TempDir()
				require.NoError(t, os.Chmod(roDir, 0o555))
				t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })
				cwdFunc = func() (string, error) { return roDir, nil }
			}

			a := agents.NewAgentWithFuncs(tt.def, os.UserHomeDir, cwdFunc)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tt.cancelCtx {
				cancel()
			}

			opts := target.InstallOpts{
				Name:      "my-plugin",
				SourceDir: srcDir,
			}

			err := a.Install(ctx, opts)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, destBase, opts.Name)
			}
		})
	}
}

func TestAgent_List(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		def     agents.AgentDef
		wantLen int
	}{
		{
			name:    "returns empty",
			def:     agents.AgentDef{Name: "cursor", Display: "Cursor", DetectHome: ".cursor"},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := agents.NewAgentWithFuncs(tt.def, os.UserHomeDir, os.Getwd)
			got, err := a.List()

			require.NoError(t, err)
			assert.Len(t, got, tt.wantLen)
		})
	}
}
