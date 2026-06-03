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

	"github.com/retr0h/agentpack/pkg/list"
	"github.com/retr0h/agentpack/pkg/outdated"
	"github.com/retr0h/agentpack/pkg/registry"
)

type fakeLister struct {
	entries       []list.Entry
	err           error
	globalEntries []list.GlobalEntry
	globalErr     error
}

func (f *fakeLister) Run() ([]list.Entry, error) {
	return f.entries, f.err
}

func (f *fakeLister) RunGlobal() ([]list.GlobalEntry, error) {
	return f.globalEntries, f.globalErr
}

type fakeOutdatedChecker struct {
	entries []outdated.Entry
	err     error
}

func (f *fakeOutdatedChecker) RunWithOptions(
	_ context.Context,
	_ outdated.Options,
) ([]outdated.Entry, error) {
	return f.entries, f.err
}

func TestListCmd(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		format     string
		lister     *fakeLister
		outdated   *fakeOutdatedChecker
		wantErr    string
		wantOutput string
		checkJSON  func(t *testing.T, output []byte)
	}{
		{
			name: "ls with no packages installed",
			args: []string{"ls"},
			lister: &fakeLister{
				entries: []list.Entry{},
			},
			wantOutput: "no plugins installed",
		},
		{
			name: "ls with text output",
			args: []string{"ls"},
			lister: &fakeLister{
				entries: []list.Entry{
					{
						Name:    "my-plugin",
						Version: "v1.0.0",
						SHA:     "abc1234",
						Source:  "github.com/org/repo",
						Targets: []list.TargetInfo{
							{Name: "claude-code", FileCount: 2},
						},
						Scope:  registry.ScopeLocal,
						Status: list.StatusOK,
					},
				},
			},
			wantOutput: "my-plugin",
		},
		{
			name:   "ls with json output",
			args:   []string{"ls"},
			format: "json",
			lister: &fakeLister{
				entries: []list.Entry{
					{
						Name:    "test-plugin",
						Version: "v2.0.0",
						SHA:     "def5678",
						Source:  "github.com/org/repo",
						Targets: []list.TargetInfo{
							{Name: "claude-code", FileCount: 1},
						},
						Scope:  registry.ScopeLocal,
						Status: list.StatusOK,
					},
				},
			},
			checkJSON: func(t *testing.T, output []byte) {
				t.Helper()
				var entries []list.Entry
				require.NoError(t, json.Unmarshal(output, &entries))
				require.Len(t, entries, 1)
				assert.Equal(t, "test-plugin", entries[0].Name)
				assert.Equal(t, "v2.0.0", entries[0].Version)
			},
		},
		{
			name: "ls --targets",
			args: []string{"ls", "--targets"},
			lister: &fakeLister{
				entries: []list.Entry{},
			},
			wantOutput: "claude-code",
		},
		{
			name: "ls --outdated",
			args: []string{"ls", "--outdated"},
			lister: &fakeLister{
				entries: []list.Entry{},
			},
			outdated: &fakeOutdatedChecker{
				entries: []outdated.Entry{
					{
						Name:         "my-plugin",
						InstalledSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
						RemoteSHA:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
						Outdated:     true,
					},
				},
			},
			wantOutput: "my-plugin",
		},
		{
			name: "ls --outdated all up to date",
			args: []string{"ls", "--outdated"},
			lister: &fakeLister{
				entries: []list.Entry{},
			},
			outdated: &fakeOutdatedChecker{
				entries: []outdated.Entry{},
			},
			wantOutput: "all plugins up to date",
		},
		{
			name: "ls --outdated error",
			args: []string{"ls", "--outdated"},
			lister: &fakeLister{
				entries: []list.Entry{},
			},
			outdated: &fakeOutdatedChecker{
				err: fmt.Errorf("network failure"),
			},
			wantErr: "network failure",
		},
		{
			name: "ls error from lister",
			args: []string{"ls"},
			lister: &fakeLister{
				err: fmt.Errorf("registry read error"),
			},
			wantErr: "registry read error",
		},
		{
			name: "ls --global with results",
			args: []string{"ls", "--global"},
			lister: &fakeLister{
				globalEntries: []list.GlobalEntry{
					{
						Agent: "claude-code",
						Skill: "my-skill",
						Dir:   "/home/user/.claude/skills",
					},
				},
			},
			wantOutput: "my-skill",
		},
		{
			name: "ls --global empty",
			args: []string{"ls", "--global"},
			lister: &fakeLister{
				globalEntries: []list.GlobalEntry{},
			},
			wantOutput: "no global plugins installed",
		},
		{
			name: "ls with contents tree",
			args: []string{"ls"},
			lister: &fakeLister{
				entries: []list.Entry{
					{
						Name:    "rich-plugin",
						Version: "v1.0.0",
						SHA:     "abc1234",
						Source:  "github.com/org/repo",
						Targets: []list.TargetInfo{
							{Name: "claude-code", FileCount: 3},
						},
						Contents: []list.ContentItem{
							{Type: "skills", Name: "intro", Targets: []string{"claude-code"}},
							{Type: "skills", Name: "outro", Targets: []string{"claude-code"}},
							{Type: "commands", Name: "deploy", Targets: []string{"claude-code"}},
						},
						Scope:  registry.ScopeLocal,
						Status: list.StatusOK,
					},
				},
			},
			wantOutput: "1 plugin installed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origLister := pkgLister
			origOutdated := pkgOutdatedChecker
			origFormat := outputFormat
			origTargets := listTargetsFlag
			origOutdatedFlag := listOutdatedFlag
			origGlobal := listGlobalFlag
			t.Cleanup(func() {
				pkgLister = origLister
				pkgOutdatedChecker = origOutdated
				outputFormat = origFormat
				listTargetsFlag = origTargets
				listOutdatedFlag = origOutdatedFlag
				listGlobalFlag = origGlobal
			})

			pkgLister = tt.lister
			if tt.outdated != nil {
				pkgOutdatedChecker = tt.outdated
			}
			if tt.format != "" {
				outputFormat = tt.format
			} else {
				outputFormat = "text"
			}
			listTargetsFlag = false
			listOutdatedFlag = false
			listGlobalFlag = false

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
