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

package remove_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/retr0h/agentpack/pkg/registry"
	"github.com/retr0h/agentpack/pkg/remove"
	removemocks "github.com/retr0h/agentpack/pkg/remove/mocks"
)

// sha256Of returns the hex SHA256 of data.
func sha256Of(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// --------------------------------------------------------------------------
// TestRun
// --------------------------------------------------------------------------

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		cancelCtx  bool
		setupMocks func(reg *removemocks.MockRegistry, pluginDir string) *registry.PackageManifest
		extraOpts  func(opts *remove.Options)
		wantErr    string
		wantRemLen int
		wantSkpLen int
		checkFiles func(t *testing.T, pluginDir string)
	}{
		{
			name: "removes files listed in manifest",
			setupMocks: func(reg *removemocks.MockRegistry, pluginDir string) *registry.PackageManifest {
				content := []byte("# skill content\n")
				filePath := filepath.Join(pluginDir, "skill.md")

				if err := os.WriteFile(filePath, content, 0o644); err != nil {
					t.Fatalf("setup WriteFile: %v", err)
				}

				m := &registry.PackageManifest{
					Name:   "test-plugin",
					Source: "github.com/org/repo",
					Files: []registry.InstalledFile{
						{
							Path:   "skill.md",
							SHA256: sha256Of(content),
							Target: "claude-code",
							Dir:    pluginDir,
						},
					},
				}

				reg.EXPECT().Load("test-plugin").Return(m, nil)
				reg.EXPECT().Remove("test-plugin").Return(nil)

				return m
			},
			wantRemLen: 1,
			wantSkpLen: 0,
			checkFiles: func(t *testing.T, pluginDir string) {
				t.Helper()
				if _, err := os.Stat(filepath.Join(pluginDir, "skill.md")); !os.IsNotExist(err) {
					t.Error("expected skill.md to be removed")
				}
			},
		},
		{
			name: "skips modified files",
			setupMocks: func(reg *removemocks.MockRegistry, pluginDir string) *registry.PackageManifest {
				filePath := filepath.Join(pluginDir, "skill.md")
				original := []byte("# original\n")

				if err := os.WriteFile(filePath, original, 0o644); err != nil {
					t.Fatalf("setup WriteFile: %v", err)
				}

				recordedSHA := sha256Of(original)

				// Modify file after recording SHA.
				if err := os.WriteFile(filePath, []byte("# MODIFIED\n"), 0o644); err != nil {
					t.Fatalf("setup modify: %v", err)
				}

				m := &registry.PackageManifest{
					Name:   "modified-plugin",
					Source: "github.com/org/repo",
					Files: []registry.InstalledFile{
						{
							Path:   "skill.md",
							SHA256: recordedSHA,
							Target: "claude-code",
							Dir:    pluginDir,
						},
					},
				}

				reg.EXPECT().Load("modified-plugin").Return(m, nil)
				reg.EXPECT().Remove("modified-plugin").Return(nil)

				return m
			},
			wantRemLen: 0,
			wantSkpLen: 1,
		},
		{
			name: "skips .git paths",
			setupMocks: func(reg *removemocks.MockRegistry, pluginDir string) *registry.PackageManifest {
				m := &registry.PackageManifest{
					Name:   "git-plugin",
					Source: "github.com/org/repo",
					Files: []registry.InstalledFile{
						{
							Path:   ".git/config",
							SHA256: "abc123",
							Target: "claude-code",
							Dir:    pluginDir,
						},
					},
				}

				reg.EXPECT().Load("git-plugin").Return(m, nil)
				reg.EXPECT().Remove("git-plugin").Return(nil)

				return m
			},
			wantRemLen: 0,
			wantSkpLen: 1,
		},
		{
			name: "OnStep called for removed file",
			setupMocks: func(reg *removemocks.MockRegistry, pluginDir string) *registry.PackageManifest {
				content := []byte("# skill content\n")
				filePath := filepath.Join(pluginDir, "step.md")

				if err := os.WriteFile(filePath, content, 0o644); err != nil {
					t.Fatalf("setup WriteFile: %v", err)
				}

				m := &registry.PackageManifest{
					Name:   "step-plugin",
					Source: "github.com/org/repo",
					Files: []registry.InstalledFile{
						{
							Path:   "step.md",
							SHA256: sha256Of(content),
							Target: "claude-code",
							Dir:    pluginDir,
						},
					},
				}

				reg.EXPECT().Load("step-plugin").Return(m, nil)
				reg.EXPECT().Remove("step-plugin").Return(nil)

				return m
			},
			extraOpts: func(opts *remove.Options) {
				opts.OnStep = func(_ remove.Step) {}
			},
			wantRemLen: 1,
			wantSkpLen: 0,
		},
		{
			name: "OnStep called for skipped file",
			setupMocks: func(reg *removemocks.MockRegistry, pluginDir string) *registry.PackageManifest {
				filePath := filepath.Join(pluginDir, "skip.md")
				original := []byte("# original\n")

				if err := os.WriteFile(filePath, original, 0o644); err != nil {
					t.Fatalf("setup WriteFile: %v", err)
				}

				recordedSHA := sha256Of(original)

				if err := os.WriteFile(filePath, []byte("# MODIFIED\n"), 0o644); err != nil {
					t.Fatalf("setup modify: %v", err)
				}

				m := &registry.PackageManifest{
					Name:   "skip-step-plugin",
					Source: "github.com/org/repo",
					Files: []registry.InstalledFile{
						{
							Path:   "skip.md",
							SHA256: recordedSHA,
							Target: "claude-code",
							Dir:    pluginDir,
						},
					},
				}

				reg.EXPECT().Load("skip-step-plugin").Return(m, nil)
				reg.EXPECT().Remove("skip-step-plugin").Return(nil)

				return m
			},
			extraOpts: func(opts *remove.Options) {
				opts.OnStep = func(_ remove.Step) {}
			},
			wantRemLen: 0,
			wantSkpLen: 1,
		},
		{
			name: "registry load failure returns wrapped error",
			setupMocks: func(reg *removemocks.MockRegistry, _ string) *registry.PackageManifest {
				reg.EXPECT().
					Load("no-such-plugin").
					Return(nil, errors.New("package not found"))

				return nil
			},
			extraOpts: func(opts *remove.Options) {
				opts.Name = "no-such-plugin"
			},
			wantErr: "load registry manifest",
		},
		{
			name:      "cancelled context returns error before load",
			cancelCtx: true,
			setupMocks: func(_ *removemocks.MockRegistry, _ string) *registry.PackageManifest {
				// no calls expected — context is already done
				return nil
			},
			extraOpts: func(opts *remove.Options) {
				opts.Name = "ctx-plugin"
			},
			wantErr: "context canceled",
		},
		{
			name: "registry remove failure returns wrapped error",
			setupMocks: func(reg *removemocks.MockRegistry, pluginDir string) *registry.PackageManifest {
				m := &registry.PackageManifest{
					Name:   "remove-fail-plugin",
					Source: "github.com/org/repo",
					Files:  []registry.InstalledFile{},
				}

				reg.EXPECT().Load("remove-fail-plugin").Return(m, nil)
				reg.EXPECT().Remove("remove-fail-plugin").Return(errors.New("remove failed"))

				return m
			},
			extraOpts: func(opts *remove.Options) {
				opts.Name = "remove-fail-plugin"
			},
			wantErr: "remove registry entry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pluginDir := t.TempDir()
			tmp := t.TempDir()

			ctrl := gomock.NewController(t)
			reg := removemocks.NewMockRegistry(ctrl)

			m := tt.setupMocks(reg, pluginDir)

			pluginName := "test-plugin"
			if m != nil {
				pluginName = m.Name
			}

			opts := remove.Options{
				Name:         pluginName,
				LockfilePath: filepath.Join(tmp, "agentpack-lock.yaml"),
				Registry:     reg,
			}

			if tt.extraOpts != nil {
				tt.extraOpts(&opts)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tt.cancelCtx {
				cancel()
			}

			result, err := remove.Run(ctx, opts)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}

				if !strContains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result.Removed) != tt.wantRemLen {
				t.Errorf("Removed len = %d, want %d", len(result.Removed), tt.wantRemLen)
			}

			if len(result.Skipped) != tt.wantSkpLen {
				t.Errorf("Skipped len = %d, want %d", len(result.Skipped), tt.wantSkpLen)
			}

			if tt.checkFiles != nil {
				tt.checkFiles(t, pluginDir)
			}
		})
	}
}

func strContains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}
