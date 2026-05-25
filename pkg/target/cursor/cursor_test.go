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

package cursor_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/retr0h/agentpack/pkg/target"
	"github.com/retr0h/agentpack/pkg/target/cursor"
)

func TestCursor_Name(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns cursor", want: "cursor"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := cursor.New()
			if got := c.Name(); got != tt.want {
				t.Errorf("Name() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCursor_DisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns Cursor", want: "Cursor"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := cursor.New()
			if got := c.DisplayName(); got != tt.want {
				t.Errorf("DisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCursor_Detect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		setupHome    func(t *testing.T) string
		wantDetected bool
	}{
		{
			name: "detects when ~/.cursor exists",
			setupHome: func(t *testing.T) string {
				t.Helper()
				home := t.TempDir()
				if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}

				return home
			},
			wantDetected: true,
		},
		{
			name: "not detected when ~/.cursor absent",
			setupHome: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantDetected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			home := tt.setupHome(t)
			c := cursor.NewWithHome(func() (string, error) { return home, nil })
			got := c.Detect()

			if got != tt.wantDetected {
				t.Errorf("Detect() = %v, want %v", got, tt.wantDetected)
			}
		})
	}
}

func TestCursor_Install(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupSrc  func(t *testing.T) string // returns source dir
		cancelCtx bool
		wantErr   string
		check     func(t *testing.T, destBase string, pluginName string)
	}{
		{
			name: "copies skills into .cursor/rules/{name}/",
			setupSrc: func(t *testing.T) string {
				t.Helper()
				src := t.TempDir()
				skillsDir := filepath.Join(src, "skills")

				if err := os.MkdirAll(skillsDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}

				if err := os.WriteFile(
					filepath.Join(skillsDir, "my-skill.md"),
					[]byte("# My Skill"),
					0o644,
				); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}

				return src
			},
			check: func(t *testing.T, destBase string, pluginName string) {
				t.Helper()
				target := filepath.Join(destBase, ".cursor", "rules", pluginName, "my-skill.md")
				if _, err := os.Stat(target); err != nil {
					t.Errorf("expected %s to exist: %v", target, err)
				}
			},
		},
		{
			name: "no skills dir is a no-op",
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
		},
		{
			name:      "cancelled context returns error",
			cancelCtx: true,
			setupSrc: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srcDir := tt.setupSrc(t)
			destBase := t.TempDir()

			// Override CWD by changing install destination via chdir.
			// Since we can't override os.Getwd() easily, use the export hook.
			c := cursor.NewWithCWD(func() (string, error) { return destBase, nil })

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tt.cancelCtx {
				cancel()
			}

			opts := target.InstallOpts{
				Name:      "my-plugin",
				SourceDir: srcDir,
			}

			err := c.Install(ctx, opts)

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

			if tt.check != nil {
				tt.check(t, destBase, opts.Name)
			}
		})
	}
}

func TestCursor_List(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantLen int
	}{
		{name: "returns empty slice", wantLen: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := cursor.New()
			got, err := c.List()

			if err != nil {
				t.Fatalf("List() error: %v", err)
			}

			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
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
