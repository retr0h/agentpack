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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/driver/agents"
	"github.com/retr0h/agentpack/pkg/target"
)

func TestDefs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		checkName string
	}{
		{
			name:      "Defs returns non-empty slice",
			checkName: "",
		},
		{
			name:      "Defs contains universal",
			checkName: "universal",
		},
		{
			name:      "Defs contains gemini-cli",
			checkName: "gemini-cli",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			defs := agents.Defs()
			assert.NotEmpty(t, defs)

			if tt.checkName == "" {
				return
			}

			found := false
			for _, d := range defs {
				if d.Name == tt.checkName {
					found = true
					break
				}
			}
			assert.True(t, found, "expected agent %q in Defs()", tt.checkName)
		})
	}
}

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
			name: "cursor",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			wantName:    "cursor",
			wantDisplay: "Cursor",
		},
		{
			name: "opencode",
			def: agents.AgentDef{
				Name:            "opencode",
				Display:         "OpenCode",
				DetectConfig:    "opencode",
				GlobalSkillsDir: ".config/opencode/skills",
			},
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
		name          string
		def           agents.AgentDef
		homeFunc      func(t *testing.T) func() (string, error)
		getenvFunc    func(t *testing.T) func(string) string
		configDirFunc func(t *testing.T) func() (string, error)
		wantDetected  bool
	}{
		{
			name: "detects via DetectHome when dir exists",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
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
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				home := t.TempDir()
				return func() (string, error) { return home, nil }
			},
			wantDetected: false,
		},
		{
			name: "home error returns false",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				return func() (string, error) { return "", errors.New("no home") }
			},
			wantDetected: false,
		},
		{
			name: "EnvOverride set to existing dir returns true",
			def: agents.AgentDef{
				Name:            "codex",
				Display:         "Codex",
				DetectHome:      ".codex",
				EnvOverride:     "CODEX_HOME",
				GlobalSkillsDir: ".codex/skills",
			},
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
			def: agents.AgentDef{
				Name:            "codex",
				Display:         "Codex",
				DetectHome:      ".codex",
				EnvOverride:     "CODEX_HOME",
				GlobalSkillsDir: ".codex/skills",
			},
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
		{
			name: "AlwaysDetect true returns true without any dir check",
			def: agents.AgentDef{
				Name:            "universal",
				Display:         "Universal",
				AlwaysDetect:    true,
				GlobalSkillsDir: ".config/agents/skills",
			},
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				return func() (string, error) { return t.TempDir(), nil }
			},
			wantDetected: true,
		},
		{
			name: "DetectConfig with dir present returns true",
			def: agents.AgentDef{
				Name:            "opencode",
				Display:         "OpenCode",
				DetectConfig:    "opencode",
				GlobalSkillsDir: ".config/opencode/skills",
			},
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				return func() (string, error) { return t.TempDir(), nil }
			},
			configDirFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				configDir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(configDir, "opencode"), 0o755))
				return func() (string, error) { return configDir, nil }
			},
			wantDetected: true,
		},
		{
			name: "DetectConfig with dir absent returns false",
			def: agents.AgentDef{
				Name:            "opencode",
				Display:         "OpenCode",
				DetectConfig:    "opencode",
				GlobalSkillsDir: ".config/opencode/skills",
			},
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				return func() (string, error) { return t.TempDir(), nil }
			},
			configDirFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				// Empty config dir — "opencode" subdir does not exist.
				configDir := t.TempDir()
				return func() (string, error) { return configDir, nil }
			},
			wantDetected: false,
		},
		{
			name: "DetectConfig with configDirFunc error returns false",
			def: agents.AgentDef{
				Name:            "opencode",
				Display:         "OpenCode",
				DetectConfig:    "opencode",
				GlobalSkillsDir: ".config/opencode/skills",
			},
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				return func() (string, error) { return t.TempDir(), nil }
			},
			configDirFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				return func() (string, error) { return "", errors.New("no config dir") }
			},
			wantDetected: false,
		},
		{
			name: "no DetectHome no DetectConfig no AlwaysDetect returns false",
			def: agents.AgentDef{
				Name:            "empty",
				Display:         "Empty",
				GlobalSkillsDir: ".agents/skills",
			},
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				return func() (string, error) { return t.TempDir(), nil }
			},
			wantDetected: false,
		},
		{
			name: "EnvOverride set but env var empty falls through to DetectHome missing",
			def: agents.AgentDef{
				Name:            "codex",
				Display:         "Codex",
				DetectHome:      ".codex",
				EnvOverride:     "CODEX_HOME",
				GlobalSkillsDir: ".codex/skills",
			},
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				home := t.TempDir()
				// .codex does NOT exist so DetectHome check fails.
				return func() (string, error) { return home, nil }
			},
			getenvFunc: func(t *testing.T) func(string) string {
				t.Helper()
				return func(_ string) string { return "" }
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

			if tt.configDirFunc != nil {
				configDir := tt.configDirFunc(t)
				a := agents.NewAgentWithConfigDir(tt.def, homeFunc, configDir)
				assert.Equal(t, tt.wantDetected, a.Detect())
				return
			}

			a := agents.NewAgentWithFuncs(tt.def, homeFunc, os.Getwd)
			assert.Equal(t, tt.wantDetected, a.Detect())
		})
	}
}

func TestAgent_SupportedTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		def       agents.AgentDef
		wantTypes []string
	}{
		{
			name: "returns skill type",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			wantTypes: []string{"skill"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := agents.NewAgentWithFuncs(tt.def, os.UserHomeDir, os.Getwd)
			assert.Equal(t, tt.wantTypes, a.SupportedTypes())
		})
	}
}

func TestAgent_Install(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		def              agents.AgentDef
		global           bool
		entries          []target.ContentEntry
		entriesFromSrc   func(src string) []target.ContentEntry
		setupSrc         func(t *testing.T) string
		setupDest        func(t *testing.T, destBase string)
		homeFunc         func(t *testing.T) func() (string, error)
		cwdFunc          func(t *testing.T, destBase string) func() (string, error)
		cancelCtx        bool
		cancelAfterDelay time.Duration
		wantErr          string
		check            func(t *testing.T, destBase string, pluginName string)
	}{
		{
			name: "local: copies skills into .agents/skills/{name}/",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
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
			name: "local: custom LocalSkillsDir is used when set",
			def: agents.AgentDef{
				Name:            "windsurf",
				Display:         "Windsurf",
				DetectHome:      ".codeium/windsurf",
				GlobalSkillsDir: ".codeium/windsurf/skills",
				LocalSkillsDir:  ".windsurf/skills",
			},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				src := t.TempDir()
				skillsDir := filepath.Join(src, "skills")
				require.NoError(t, os.MkdirAll(skillsDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(skillsDir, "ws-skill.md"),
					[]byte("# WS Skill"),
					0o644,
				))
				return src
			},
			check: func(t *testing.T, destBase string, pluginName string) {
				t.Helper()
				tgt := filepath.Join(destBase, ".windsurf", "skills", pluginName, "ws-skill.md")
				_, err := os.Stat(tgt)
				assert.NoError(t, err)
			},
		},
		{
			name: "global: copies skills into GlobalSkillsDir/{name}/ under home",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			global: true,
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
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				home := t.TempDir()
				return func() (string, error) { return home, nil }
			},
			check: func(t *testing.T, destBase string, pluginName string) {
				t.Helper()
				tgt := filepath.Join(destBase, ".cursor", "skills", pluginName, "my-skill.md")
				_, err := os.Stat(tgt)
				assert.NoError(t, err)
			},
		},
		{
			name: "global: home dir error propagates",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			global: true,
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				return func() (string, error) { return "", errors.New("no home") }
			},
			wantErr: "home dir",
		},
		{
			name: "local: no skills dir is a no-op",
			def: agents.AgentDef{
				Name:            "gemini",
				Display:         "Gemini CLI",
				DetectHome:      ".gemini",
				GlobalSkillsDir: ".gemini/skills",
			},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
		},
		{
			name: "cancelled context returns error",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			cancelCtx: true,
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: "context canceled",
		},
		{
			name: "local: cwdFunc error propagates",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: "getwd",
		},
		{
			name: "local: mkdirAll failure propagates error",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: "mkdir agents skills dir",
		},
		{
			name: "copyFile: unreadable source file returns error",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				src := t.TempDir()
				skillsDir := filepath.Join(src, "skills")
				require.NoError(t, os.MkdirAll(skillsDir, 0o755))
				secretFile := filepath.Join(skillsDir, "secret.md")
				require.NoError(t, os.WriteFile(secretFile, []byte("secret"), 0o000))
				t.Cleanup(func() { _ = os.Chmod(secretFile, 0o644) })
				return src
			},
			wantErr: "copy skills: read",
		},
		{
			name: "copyFile: read-only destination dir causes write error",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				src := t.TempDir()
				skillsDir := filepath.Join(src, "skills")
				require.NoError(t, os.MkdirAll(skillsDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(skillsDir, "skill.md"),
					[]byte("# Skill"),
					0o644,
				))
				return src
			},
			wantErr: "copy skills: write",
		},
		{
			name: "local: unreadable file in destDir causes enumerate error",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				// Skills dir is empty so copyTreeIfExists is a no-op.
				return t.TempDir()
			},
			setupDest: func(t *testing.T, destBase string) {
				t.Helper()
				destDir := filepath.Join(destBase, ".agents", "skills", "my-plugin")
				require.NoError(t, os.MkdirAll(destDir, 0o755))
				unreadable := filepath.Join(destDir, "unreadable.md")
				require.NoError(t, os.WriteFile(unreadable, []byte("secret"), 0o000))
				t.Cleanup(func() { _ = os.Chmod(unreadable, 0o644) })
			},
			wantErr: "enumerate installed files",
		},
		{
			name: "entries: installs only listed entries, not others",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "k8s", Type: "skill", Root: filepath.Join(src, "skills", "k8s")},
				}
			},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				src := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(src, "skills", "k8s"), 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(src, "skills", "k8s", "SKILL.md"),
					[]byte("# K8s Skill"),
					0o644,
				))
				require.NoError(t, os.MkdirAll(filepath.Join(src, "skills", "react"), 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(src, "skills", "react", "SKILL.md"),
					[]byte("# React Skill"),
					0o644,
				))
				return src
			},
			check: func(t *testing.T, destBase string, _ string) {
				t.Helper()
				k8sFile := filepath.Join(destBase, ".agents", "skills", "k8s", "SKILL.md")
				_, err := os.Stat(k8sFile)
				assert.NoError(t, err)
				reactFile := filepath.Join(destBase, ".agents", "skills", "react", "SKILL.md")
				_, err = os.Stat(reactFile)
				assert.True(t, os.IsNotExist(err))
			},
		},
		{
			name: "entries: global installs into GlobalSkillsDir under home",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			global: true,
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "k8s", Type: "skill", Root: filepath.Join(src, "skills", "k8s")},
				}
			},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				src := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(src, "skills", "k8s"), 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(src, "skills", "k8s", "SKILL.md"),
					[]byte("# K8s Skill"),
					0o644,
				))
				return src
			},
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				home := t.TempDir()
				return func() (string, error) { return home, nil }
			},
			check: func(t *testing.T, destBase string, _ string) {
				t.Helper()
				k8sFile := filepath.Join(destBase, ".cursor", "skills", "k8s", "SKILL.md")
				_, err := os.Stat(k8sFile)
				assert.NoError(t, err)
			},
		},
		{
			name: "entries: global home error propagates",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			global: true,
			entries: []target.ContentEntry{
				{Name: "k8s", Type: "skill", Root: "/does-not-matter"},
			},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			homeFunc: func(t *testing.T) func() (string, error) {
				t.Helper()
				return func() (string, error) { return "", errors.New("no home") }
			},
			wantErr: "home dir",
		},
		{
			name: "entries: local cwdFunc error propagates",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			entries: []target.ContentEntry{
				{Name: "k8s", Type: "skill", Root: "/does-not-matter"},
			},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: "getwd",
		},
		{
			name: "entries: local custom LocalSkillsDir is used",
			def: agents.AgentDef{
				Name:            "windsurf",
				Display:         "Windsurf",
				DetectHome:      ".codeium/windsurf",
				GlobalSkillsDir: ".codeium/windsurf/skills",
				LocalSkillsDir:  ".windsurf/skills",
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{
						Name: "ws-skill",
						Type: "skill",
						Root: filepath.Join(src, "skills", "ws-skill"),
					},
				}
			},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				src := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(src, "skills", "ws-skill"), 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(src, "skills", "ws-skill", "SKILL.md"),
					[]byte("# WS Skill"),
					0o644,
				))
				return src
			},
			check: func(t *testing.T, destBase string, _ string) {
				t.Helper()
				tgt := filepath.Join(destBase, ".windsurf", "skills", "ws-skill", "SKILL.md")
				_, err := os.Stat(tgt)
				assert.NoError(t, err)
			},
		},
		{
			name: "entries: mkdirAll failure propagates",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			entries: []target.ContentEntry{
				{Name: "k8s", Type: "skill", Root: "/does-not-matter"},
			},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: "mkdir agents skills dir",
		},
		{
			name: "entries: cancelled context mid-loop returns error",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			// Two entries: the first processes successfully (empty root = no-op
			// copyTreeIfExists). time.AfterFunc fires cancel during the syscalls of
			// entry-0 processing so that entry-1's ctx.Err() check catches it.
			entries: []target.ContentEntry{
				{Name: "first", Type: "skill"},
				{Name: "second", Type: "skill"},
			},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			cancelAfterDelay: time.Nanosecond,
			wantErr:          "context canceled",
		},
		{
			name: "entries: unreadable file in destDir causes enumerate error",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			entries: []target.ContentEntry{
				{Name: "k8s", Type: "skill"},
			},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			setupDest: func(t *testing.T, destBase string) {
				t.Helper()
				destDir := filepath.Join(destBase, ".agents", "skills", "k8s")
				require.NoError(t, os.MkdirAll(destDir, 0o755))
				unreadable := filepath.Join(destDir, "unreadable.md")
				require.NoError(t, os.WriteFile(unreadable, []byte("secret"), 0o000))
				t.Cleanup(func() { _ = os.Chmod(unreadable, 0o644) })
			},
			wantErr: "enumerate installed files",
		},
		{
			name: "entries: copy skills error propagates",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
			entriesFromSrc: func(src string) []target.ContentEntry {
				return []target.ContentEntry{
					{Name: "k8s", Type: "skill", Root: filepath.Join(src, "skills", "k8s")},
				}
			},
			setupSrc: func(t *testing.T) string {
				t.Helper()
				src := t.TempDir()
				skillsK8s := filepath.Join(src, "skills", "k8s")
				require.NoError(t, os.MkdirAll(skillsK8s, 0o755))
				secretFile := filepath.Join(skillsK8s, "secret.md")
				require.NoError(t, os.WriteFile(secretFile, []byte("secret"), 0o000))
				t.Cleanup(func() { _ = os.Chmod(secretFile, 0o644) })
				return src
			},
			wantErr: "copy skills: read",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir := tt.setupSrc(t)
			destBase := t.TempDir()

			homeFunc := func() (string, error) { return destBase, nil }
			if tt.homeFunc != nil {
				homeFunc = tt.homeFunc(t)
			}

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
			if tt.wantErr == "copy skills: write" {
				// Pre-create the destDir so MkdirAll succeeds, then make it read-only
				// so copyFile's WriteFile call fails.
				roDestDir := filepath.Join(destBase, ".agents", "skills", "my-plugin")
				require.NoError(t, os.MkdirAll(roDestDir, 0o755))
				require.NoError(t, os.Chmod(roDestDir, 0o555))
				t.Cleanup(func() { _ = os.Chmod(roDestDir, 0o755) })
				cwdFunc = func() (string, error) { return destBase, nil }
			}
			if tt.setupDest != nil {
				tt.setupDest(t, destBase)
			}

			a := agents.NewAgentWithFuncs(tt.def, homeFunc, cwdFunc)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tt.cancelCtx {
				cancel()
			}
			if tt.cancelAfterDelay > 0 {
				time.AfterFunc(tt.cancelAfterDelay, cancel)
			}

			entries := tt.entries
			if tt.entriesFromSrc != nil {
				entries = tt.entriesFromSrc(srcDir)
			}
			opts := target.InstallOpts{
				Name:      "my-plugin",
				SourceDir: srcDir,
				Global:    tt.global,
				Entries:   entries,
			}

			files, err := a.Install(ctx, opts)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			_ = files

			if tt.check != nil {
				checkBase := destBase
				if tt.homeFunc != nil {
					home, _ := homeFunc()
					checkBase = home
				}
				tt.check(t, checkBase, opts.Name)
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
			name: "returns empty",
			def: agents.AgentDef{
				Name:            "cursor",
				Display:         "Cursor",
				DetectHome:      ".cursor",
				GlobalSkillsDir: ".cursor/skills",
			},
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
