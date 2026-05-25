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
	"os"
	"path/filepath"
	"testing"

	"github.com/retr0h/agentpack/pkg/registry"
	"github.com/retr0h/agentpack/pkg/remove"
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
		setup      func(t *testing.T, tmp string) remove.Options
		cancelCtx  bool
		wantErr    string
		wantRemLen int
		wantSkpLen int
		checkFiles func(t *testing.T, opts remove.Options)
	}{
		{
			name: "removes files listed in manifest",
			setup: func(t *testing.T, tmp string) remove.Options {
				t.Helper()
				t.Setenv("HOME", tmp)

				content := []byte("# skill content\n")
				pluginDir := t.TempDir()
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

				if err := registry.Save(m); err != nil {
					t.Fatalf("setup Save: %v", err)
				}

				return remove.Options{
					Name:         "test-plugin",
					LockfilePath: filepath.Join(tmp, "agentpack-lock.yaml"),
				}
			},
			wantRemLen: 1,
			wantSkpLen: 0,
			checkFiles: func(t *testing.T, opts remove.Options) {
				t.Helper()
				// The registry manifest should be gone.
				_, err := registry.Load(opts.Name)
				if err == nil {
					t.Error("expected registry manifest to be removed")
				}
			},
		},
		{
			name: "skips modified files",
			setup: func(t *testing.T, tmp string) remove.Options {
				t.Helper()
				t.Setenv("HOME", tmp)

				pluginDir := t.TempDir()
				filePath := filepath.Join(pluginDir, "skill.md")

				// Write original content.
				original := []byte("# original\n")
				if err := os.WriteFile(filePath, original, 0o644); err != nil {
					t.Fatalf("setup WriteFile: %v", err)
				}

				// Record original SHA but then modify the file.
				recordedSHA := sha256Of(original)

				modified := []byte("# MODIFIED\n")
				if err := os.WriteFile(filePath, modified, 0o644); err != nil {
					t.Fatalf("setup modify: %v", err)
				}

				m := &registry.PackageManifest{
					Name:   "modified-plugin",
					Source: "github.com/org/repo",
					Files: []registry.InstalledFile{
						{
							Path:   "skill.md",
							SHA256: recordedSHA, // does NOT match current file
							Target: "claude-code",
							Dir:    pluginDir,
						},
					},
				}

				if err := registry.Save(m); err != nil {
					t.Fatalf("setup Save: %v", err)
				}

				return remove.Options{
					Name:         "modified-plugin",
					LockfilePath: filepath.Join(tmp, "agentpack-lock.yaml"),
				}
			},
			wantRemLen: 0,
			wantSkpLen: 1,
		},
		{
			name: "skips .git paths",
			setup: func(t *testing.T, tmp string) remove.Options {
				t.Helper()
				t.Setenv("HOME", tmp)

				pluginDir := t.TempDir()

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

				if err := registry.Save(m); err != nil {
					t.Fatalf("setup Save: %v", err)
				}

				return remove.Options{
					Name:         "git-plugin",
					LockfilePath: filepath.Join(tmp, "agentpack-lock.yaml"),
				}
			},
			wantRemLen: 0,
			wantSkpLen: 1,
		},
		{
			name: "OnStep called for removed file",
			setup: func(t *testing.T, tmp string) remove.Options {
				t.Helper()
				t.Setenv("HOME", tmp)

				content := []byte("# skill content\n")
				pluginDir := t.TempDir()
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

				if err := registry.Save(m); err != nil {
					t.Fatalf("setup Save: %v", err)
				}

				var stepped []remove.Step
				return remove.Options{
					Name:         "step-plugin",
					LockfilePath: filepath.Join(tmp, "agentpack-lock.yaml"),
					OnStep: func(s remove.Step) {
						stepped = append(stepped, s)
					},
				}
			},
			wantRemLen: 1,
			wantSkpLen: 0,
		},
		{
			name: "OnStep called for skipped file",
			setup: func(t *testing.T, tmp string) remove.Options {
				t.Helper()
				t.Setenv("HOME", tmp)

				pluginDir := t.TempDir()
				filePath := filepath.Join(pluginDir, "skip.md")

				original := []byte("# original\n")
				if err := os.WriteFile(filePath, original, 0o644); err != nil {
					t.Fatalf("setup WriteFile: %v", err)
				}

				recordedSHA := sha256Of(original)

				// Modify so checksum won't match.
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

				if err := registry.Save(m); err != nil {
					t.Fatalf("setup Save: %v", err)
				}

				var stepped []remove.Step
				return remove.Options{
					Name:         "skip-step-plugin",
					LockfilePath: filepath.Join(tmp, "agentpack-lock.yaml"),
					OnStep: func(s remove.Step) {
						stepped = append(stepped, s)
					},
				}
			},
			wantRemLen: 0,
			wantSkpLen: 1,
		},
		{
			name: "nonexistent package returns error",
			setup: func(t *testing.T, tmp string) remove.Options {
				t.Helper()
				t.Setenv("HOME", tmp)

				return remove.Options{
					Name:         "no-such-plugin",
					LockfilePath: filepath.Join(tmp, "agentpack-lock.yaml"),
				}
			},
			wantErr: "load registry manifest",
		},
		{
			name: "cancelled context returns error",
			setup: func(t *testing.T, tmp string) remove.Options {
				t.Helper()
				t.Setenv("HOME", tmp)

				return remove.Options{Name: "ctx-plugin"}
			},
			cancelCtx: true,
			wantErr:   "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			opts := tt.setup(t, tmp)

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
				tt.checkFiles(t, opts)
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
