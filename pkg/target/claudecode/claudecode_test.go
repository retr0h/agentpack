package claudecode_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/retr0h/agentpack/pkg/target"
	"github.com/retr0h/agentpack/pkg/target/claudecode"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInstall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (srcDir string, installDir string)
		wantErr string
		check   func(t *testing.T, installDir string)
	}{
		{
			name: "copies skills recursively to .claude/skills/",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "skills", "review", "SKILL.md"), "# Review")
				return src, t.TempDir()
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				p := filepath.Join(dir, ".claude", "skills", "review", "SKILL.md")
				if _, err := os.Stat(p); err != nil {
					t.Errorf("skill not installed: %v", err)
				}
			},
		},
		{
			name: "copies commands to .claude/commands/",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				src := t.TempDir()
				writeFile(t, filepath.Join(src, "commands", "scan.md"), "# Scan")
				return src, t.TempDir()
			},
			check: func(t *testing.T, dir string) {
				t.Helper()
				if _, err := os.Stat(filepath.Join(dir, ".claude", "commands", "scan.md")); err != nil {
					t.Errorf("command not installed: %v", err)
				}
			},
		},
		{
			name: "skips missing content dirs without error",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), t.TempDir()
			},
		},
		{
			name: "fails on cancelled context",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), t.TempDir()
			},
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srcDir, installDir := tt.setup(t)
			cc := claudecode.New()
			ctx := context.Background()
			if tt.wantErr == "context canceled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err := cc.Install(ctx, target.InstallOpts{
				Name: "test", SourceDir: srcDir, Dir: installDir,
			})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, installDir)
			}
		})
	}
}

func TestShortSHA(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, sha, want string }{
		{"full", "abc1234567890", "abc1234"},
		{"short", "abc", "abc"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := claudecode.ShortSHA(tt.sha); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatDate(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, ts, want string }{
		{"trims time", "2026-05-24T14:00:00Z", "2026-05-24"},
		{"no T", "2026-05-24", "2026-05-24"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := claudecode.FormatDate(tt.ts); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
