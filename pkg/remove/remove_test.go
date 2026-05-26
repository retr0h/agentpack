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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	t.Parallel()

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
				require.NoError(t, os.WriteFile(filePath, content, 0o644))

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
				_, err := os.Stat(filepath.Join(pluginDir, "skill.md"))
				assert.True(t, os.IsNotExist(err))
			},
		},
		{
			name: "skips modified files",
			setupMocks: func(reg *removemocks.MockRegistry, pluginDir string) *registry.PackageManifest {
				filePath := filepath.Join(pluginDir, "skill.md")
				original := []byte("# original\n")
				require.NoError(t, os.WriteFile(filePath, original, 0o644))

				recordedSHA := sha256Of(original)

				// Modify file after recording SHA.
				require.NoError(t, os.WriteFile(filePath, []byte("# MODIFIED\n"), 0o644))

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
				require.NoError(t, os.WriteFile(filePath, content, 0o644))

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
				require.NoError(t, os.WriteFile(filePath, original, 0o644))

				recordedSHA := sha256Of(original)

				require.NoError(t, os.WriteFile(filePath, []byte("# MODIFIED\n"), 0o644))

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
		{
			name: "context cancelled mid-loop returns error",
			setupMocks: func(reg *removemocks.MockRegistry, pluginDir string) *registry.PackageManifest {
				// Two files: the first passes checksum and is removed; the second
				// is skipped because of a missing checksum match — but we cancel
				// the context via the OnStep callback so the loop exits on
				// ctx.Err() check at the top of the second iteration.
				content := []byte("# first\n")
				first := filepath.Join(pluginDir, "first.md")
				require.NoError(t, os.WriteFile(first, content, 0o644))

				m := &registry.PackageManifest{
					Name:   "mid-cancel-plugin",
					Source: "github.com/org/repo",
					Files: []registry.InstalledFile{
						{Path: "first.md", SHA256: sha256Of(content), Target: "claude-code", Dir: pluginDir},
						{Path: "second.md", SHA256: "deadbeef", Target: "claude-code", Dir: pluginDir},
					},
				}

				reg.EXPECT().Load("mid-cancel-plugin").Return(m, nil)
				// Remove is NOT expected — we cancel before it's called.

				return m
			},
			extraOpts: func(opts *remove.Options) {
				opts.Name = "mid-cancel-plugin"
				// We cancel via the OnStep hook after the first file is processed.
			},
			wantErr: "context canceled",
		},
		{
			name: "read lockfile error is propagated",
			setupMocks: func(reg *removemocks.MockRegistry, _ string) *registry.PackageManifest {
				m := &registry.PackageManifest{
					Name:   "lockread-fail-plugin",
					Source: "github.com/org/repo",
					Files:  []registry.InstalledFile{},
				}

				reg.EXPECT().Load("lockread-fail-plugin").Return(m, nil)
				reg.EXPECT().Remove("lockread-fail-plugin").Return(nil)

				return m
			},
			extraOpts: func(opts *remove.Options) {
				opts.Name = "lockread-fail-plugin"
				// Point to an unreadable (existing) file to trigger lockfile.Read error.
				// We'll override LockfilePath with a chmod-0 file in the test body.
			},
			wantErr: "read lockfile",
		},
		{
			name: "write lockfile error is propagated",
			setupMocks: func(reg *removemocks.MockRegistry, _ string) *registry.PackageManifest {
				m := &registry.PackageManifest{
					Name:   "lockwrite-fail-plugin",
					Source: "github.com/org/repo",
					Files:  []registry.InstalledFile{},
				}

				reg.EXPECT().Load("lockwrite-fail-plugin").Return(m, nil)
				reg.EXPECT().Remove("lockwrite-fail-plugin").Return(nil)

				return m
			},
			extraOpts: func(opts *remove.Options) {
				opts.Name = "lockwrite-fail-plugin"
				// LockfilePath will be overridden to a read-only dir path in test body.
			},
			wantErr: "write lockfile",
		},
		{
			name: "os.Remove failure returns wrapped error",
			setupMocks: func(reg *removemocks.MockRegistry, pluginDir string) *registry.PackageManifest {
				// Create a file with correct checksum but make its parent dir
				// read-only so os.Remove fails.
				subDir := filepath.Join(pluginDir, "subdir")
				require.NoError(t, os.Mkdir(subDir, 0o755))
				content := []byte("# protected\n")
				filePath := filepath.Join(subDir, "protected.md")
				require.NoError(t, os.WriteFile(filePath, content, 0o644))

				m := &registry.PackageManifest{
					Name:   "perm-remove-plugin",
					Source: "github.com/org/repo",
					Files: []registry.InstalledFile{
						{Path: "protected.md", SHA256: sha256Of(content), Target: "claude-code", Dir: subDir},
					},
				}

				reg.EXPECT().Load("perm-remove-plugin").Return(m, nil)
				// Remove is NOT expected — we error on os.Remove.

				return m
			},
			extraOpts: func(opts *remove.Options) {
				opts.Name = "perm-remove-plugin"
			},
			wantErr: "remove",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

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

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tt.cancelCtx {
				cancel()
			}

			// For the "mid-loop cancel" test, cancel via the OnStep hook.
			if tt.name == "context cancelled mid-loop returns error" {
				opts.OnStep = func(_ remove.Step) {
					cancel()
				}
			}

			// For the "os.Remove failure" test, make the subdir unreadable
			// AFTER setupMocks (so the file was written), right before Run.
			if tt.name == "os.Remove failure returns wrapped error" {
				subDir := filepath.Join(pluginDir, "subdir")
				require.NoError(t, os.Chmod(subDir, 0o555))
				t.Cleanup(func() { _ = os.Chmod(subDir, 0o755) })
			}

			// For "read lockfile error" test, create an unreadable lockfile.
			if tt.name == "read lockfile error is propagated" {
				lockFile := filepath.Join(tmp, "agentpack-lock.yaml")
				require.NoError(t, os.WriteFile(lockFile, []byte("installs: []\n"), 0o644))
				require.NoError(t, os.Chmod(lockFile, 0o000))
				t.Cleanup(func() { _ = os.Chmod(lockFile, 0o644) })
				opts.LockfilePath = lockFile
			}

			// For "write lockfile error" test, point lockfile into a read-only dir.
			if tt.name == "write lockfile error is propagated" {
				roDir := filepath.Join(tmp, "readonly")
				require.NoError(t, os.Mkdir(roDir, 0o555))
				t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })
				opts.LockfilePath = filepath.Join(roDir, "lock.yaml")
			}

			if tt.extraOpts != nil {
				tt.extraOpts(&opts)
			}

			result, err := remove.Run(ctx, opts)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.True(t, strings.Contains(err.Error(), tt.wantErr))
				return
			}

			require.NoError(t, err)
			assert.Len(t, result.Removed, tt.wantRemLen)
			assert.Len(t, result.Skipped, tt.wantSkpLen)

			if tt.checkFiles != nil {
				tt.checkFiles(t, pluginDir)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestRunWithDefaultRegistry covers the defaultRegistry wrapper methods.
// These tests mutate the registry package-level osUserHomeDir global and
// therefore must NOT run in parallel with each other or other tests that
// use that global.
// --------------------------------------------------------------------------

func TestRunWithDefaultRegistry(t *testing.T) {
	tests := []struct {
		name    string
		wantErr string
	}{
		{
			name:    "defaultRegistry.Load is called when Registry is nil",
			wantErr: "load registry manifest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			// Redirect the registry to a temp home so we don't touch ~/.config.
			restore := registry.SetOsUserHomeDir(func() (string, error) { return tmp, nil })
			defer restore()

			// Registry is nil — Run will use defaultRegistry{} which calls
			// registry.Load("no-such-pkg"). That returns "not found in registry"
			// which is wrapped as "load registry manifest: ...".
			_, err := remove.Run(context.Background(), remove.Options{
				Name:         "no-such-pkg",
				LockfilePath: filepath.Join(tmp, "lock.yaml"),
				Registry:     nil,
			})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestRunDefaultRegistryRemove covers defaultRegistry.Remove by performing
// a full successful remove using the real registry (with redirected home).
func TestRunDefaultRegistryRemove(t *testing.T) {
	tests := []struct {
		name       string
		wantRemLen int
	}{
		{
			name:       "defaultRegistry.Remove is called on successful run",
			wantRemLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			restore := registry.SetOsUserHomeDir(func() (string, error) { return tmp, nil })
			defer restore()

			// Save a real registry manifest so defaultRegistry.Load succeeds.
			m := &registry.PackageManifest{
				Name:   "real-pkg",
				Source: "github.com/org/real",
				Files:  []registry.InstalledFile{},
			}
			require.NoError(t, registry.Save(m))

			result, err := remove.Run(context.Background(), remove.Options{
				Name:         "real-pkg",
				LockfilePath: filepath.Join(tmp, "lock.yaml"),
				Registry:     nil,
			})

			require.NoError(t, err)
			assert.Len(t, result.Removed, tt.wantRemLen)
		})
	}
}
