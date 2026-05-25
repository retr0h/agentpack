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

package install_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/retr0h/agentpack/pkg/install"
)

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

// listArchiveEntries opens a .agentpack (gzipped tar) and returns all entry
// names inside it.
func listArchiveEntries(t *testing.T, archivePath string) []string {
	t.Helper()

	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer func() { _ = f.Close() }()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)
	var names []string

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		names = append(names, hdr.Name)
	}

	return names
}

// makeSkillRepo creates a temp directory that looks like the agentskills.io
// layout: skills/{name}/SKILL.md.
func makeSkillRepo(t *testing.T, skills []string) string {
	t.Helper()
	dir := t.TempDir()

	for _, s := range skills {
		skillDir := filepath.Join(dir, "skills", s)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatalf("mkdir skill %s: %v", s, err)
		}
		if err := os.WriteFile(
			filepath.Join(skillDir, "SKILL.md"),
			[]byte("# "+s+"\n"),
			0o644,
		); err != nil {
			t.Fatalf("write SKILL.md: %v", err)
		}
	}

	return dir
}

// makeRepoWithContent creates a temp directory with a mix of content and noise.
func makeRepoWithContent(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Recognized content.
	skillDir := filepath.Join(dir, "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# skill\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	cmdDir := filepath.Join(dir, "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatalf("mkdir commands: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cmdDir, "scan.md"), []byte("# scan\n"), 0o644); err != nil {
		t.Fatalf("write scan.md: %v", err)
	}

	// Noise that should never appear in the archive.
	for _, noise := range []string{".github", ".git", "tools", "scripts"} {
		noiseDir := filepath.Join(dir, noise)
		if err := os.MkdirAll(noiseDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", noise, err)
		}
		if err := os.WriteFile(filepath.Join(noiseDir, "file.txt"), []byte("noise\n"), 0o644); err != nil {
			t.Fatalf("write noise: %v", err)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("readme\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}

	return dir
}

// --------------------------------------------------------------------------
// TestAutoPackage
// --------------------------------------------------------------------------

func TestAutoPackage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		setup        func(t *testing.T) string
		sha          string
		skillFilter  []string
		agentFilter  []string
		cancelCtx    bool
		wantErr      string
		checkEntries func(t *testing.T, entries []string)
	}{
		{
			name: "packages recognized content dirs only",
			setup: func(t *testing.T) string {
				t.Helper()
				return makeRepoWithContent(t)
			},
			sha: "abc1234567890",
			checkEntries: func(t *testing.T, entries []string) {
				t.Helper()

				// Must contain content files.
				wantContains := []string{
					"skills/my-skill/SKILL.md",
					"commands/scan.md",
					".agentpack/metadata.json",
					".agentpack/checksums.txt",
				}
				for _, want := range wantContains {
					found := false
					for _, e := range entries {
						if e == want {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("archive missing entry %q; got: %v", want, entries)
					}
				}

				// Must NOT contain noise dirs.
				for _, e := range entries {
					for _, bad := range []string{".github", ".git/", "tools/", "scripts/", "README.md"} {
						if strings.HasPrefix(e, bad) || e == bad {
							t.Errorf("archive contains excluded entry %q", e)
						}
					}
				}
			},
		},
		{
			name: "skill filter restricts to named subdirs only",
			setup: func(t *testing.T) string {
				t.Helper()
				return makeSkillRepo(t, []string{"review", "deploy", "scan"})
			},
			sha:         "deadbeef00000",
			skillFilter: []string{"review", "scan"},
			checkEntries: func(t *testing.T, entries []string) {
				t.Helper()

				// review and scan must be present.
				for _, want := range []string{
					"skills/review/SKILL.md",
					"skills/scan/SKILL.md",
				} {
					found := false
					for _, e := range entries {
						if e == want {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("missing expected entry %q; got: %v", want, entries)
					}
				}

				// deploy must be absent.
				for _, e := range entries {
					if strings.HasPrefix(e, "skills/deploy/") {
						t.Errorf("deploy skill should be filtered out but found entry %q", e)
					}
				}
			},
		},
		{
			name: "empty clone dir produces archive with only metadata",
			setup: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			sha: "cafebabe00000",
			checkEntries: func(t *testing.T, entries []string) {
				t.Helper()

				for _, want := range []string{
					".agentpack/metadata.json",
					".agentpack/checksums.txt",
				} {
					found := false
					for _, e := range entries {
						if e == want {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("missing %q in entries: %v", want, entries)
					}
				}
			},
		},
		{
			name: "skills dir with flat files (no subdirs) is packaged whole",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				skillsDir := filepath.Join(dir, "skills")
				if err := os.MkdirAll(skillsDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(skillsDir, "flat.md"), []byte("# flat\n"), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
				return dir
			},
			sha: "flat1234",
			checkEntries: func(t *testing.T, entries []string) {
				t.Helper()

				found := false
				for _, e := range entries {
					if e == "skills/flat.md" {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("flat file missing from archive; entries: %v", entries)
				}
			},
		},
		{
			name: "returns error when context is already cancelled",
			setup: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			sha:       "abc",
			cancelCtx: true,
			wantErr:   "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setup(t)

			var ctx context.Context
			var cancel context.CancelFunc

			if tt.cancelCtx {
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			} else {
				ctx = context.Background()
				cancel = func() {}
			}

			defer cancel()

			archivePath, err := install.AutoPackage(ctx, dir, "test-pkg", tt.sha, tt.skillFilter, tt.agentFilter)

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

			defer func() { _ = os.Remove(archivePath) }()

			if archivePath == "" {
				t.Fatal("archivePath is empty")
			}

			if _, statErr := os.Stat(archivePath); statErr != nil {
				t.Fatalf("archive not on disk: %v", statErr)
			}

			if tt.checkEntries != nil {
				entries := listArchiveEntries(t, archivePath)
				tt.checkEntries(t, entries)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestFilteredSubdirs
// --------------------------------------------------------------------------

func TestFilteredSubdirs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		filter      []string
		wantNames   []string
		wantMissing []string
	}{
		{
			name: "returns all subdirs when filter is empty",
			setup: func(t *testing.T) string {
				t.Helper()
				return makeSkillRepo(t, []string{"alpha", "beta", "gamma"})
			},
			filter:    nil,
			wantNames: []string{"alpha", "beta", "gamma"},
		},
		{
			name: "restricts to filter names when filter is set",
			setup: func(t *testing.T) string {
				t.Helper()
				return makeSkillRepo(t, []string{"alpha", "beta", "gamma"})
			},
			filter:      []string{"alpha", "gamma"},
			wantNames:   []string{"alpha", "gamma"},
			wantMissing: []string{"beta"},
		},
		{
			name: "returns content dir itself when only files present (flat layout)",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				skillsDir := filepath.Join(dir, "skills")
				if err := os.MkdirAll(skillsDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(skillsDir, "flat.md"), []byte("x"), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
				return dir
			},
			filter:    []string{"anything"},
			wantNames: []string{"skills"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setup(t)
			contentDir := filepath.Join(dir, "skills")

			roots := install.FilteredSubdirs(contentDir, tt.filter)

			// Verify wanted names appear in the returned roots.
			for _, want := range tt.wantNames {
				found := false
				for _, root := range roots {
					if filepath.Base(root) == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("want subdir %q in roots %v", want, roots)
				}
			}

			// Verify missing names do not appear.
			for _, missing := range tt.wantMissing {
				for _, root := range roots {
					if filepath.Base(root) == missing {
						t.Errorf("subdir %q should be absent but found in roots %v", missing, roots)
					}
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestStoreArchive
// --------------------------------------------------------------------------

func TestStoreArchive(t *testing.T) {
	// NOTE: not parallel — subtests mutate shared package-level state.

	tests := []struct {
		name    string
		setup   func(t *testing.T) (srcPath, archivesDir string)
		pkgName string
		sha     string
		wantErr string
		check   func(t *testing.T, dstPath string)
	}{
		{
			name: "copies archive to store with correct name",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "src.agentpack")
				if err := os.WriteFile(src, []byte("agentpack data"), 0o644); err != nil {
					t.Fatalf("write src: %v", err)
				}
				storeDir := filepath.Join(dir, "archives")
				if err := os.MkdirAll(storeDir, 0o700); err != nil {
					t.Fatalf("mkdir store: %v", err)
				}
				return src, storeDir
			},
			pkgName: "my-plugin",
			sha:     "abc1234567890abcdef",
			check: func(t *testing.T, dstPath string) {
				t.Helper()
				data, err := os.ReadFile(dstPath)
				if err != nil {
					t.Fatalf("read dst: %v", err)
				}
				if string(data) != "agentpack data" {
					t.Errorf("content = %q, want %q", string(data), "agentpack data")
				}
				base := filepath.Base(dstPath)
				if !strings.HasPrefix(base, "my-plugin@") || !strings.HasSuffix(base, ".agentpack") {
					t.Errorf("unexpected archive name %q", base)
				}
			},
		},
		{
			name: "returns error when source does not exist",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				storeDir := filepath.Join(dir, "archives")
				if err := os.MkdirAll(storeDir, 0o700); err != nil {
					t.Fatalf("mkdir store: %v", err)
				}
				return filepath.Join(dir, "nonexistent.agentpack"), storeDir
			},
			pkgName: "fail-plugin",
			sha:     "abc1234",
			wantErr: "open",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Not parallel: mutates archivesDirFunc package-level var.

			srcPath, storeDir := tt.setup(t)

			// Override archivesDir via the exported test hook.
			restore := install.SetArchivesDir(func() (string, error) { return storeDir, nil })
			defer restore()

			dstPath, err := install.StoreArchive(srcPath, tt.pkgName, tt.sha)

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

			if tt.check != nil {
				tt.check(t, dstPath)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestArchivesDir
// --------------------------------------------------------------------------

func TestArchivesDir(t *testing.T) {
	// NOTE: not parallel — subtests mutate shared package-level state.

	tests := []struct {
		name    string
		homeDir string
		wantErr string
		check   func(t *testing.T, dir string)
	}{
		{
			name:    "creates archives dir under home",
			homeDir: t.TempDir(),
			check: func(t *testing.T, dir string) {
				t.Helper()
				info, err := os.Stat(dir)
				if err != nil {
					t.Fatalf("stat dir: %v", err)
				}
				if !info.IsDir() {
					t.Error("expected directory")
				}
				if !strings.HasSuffix(dir, filepath.Join(".config", "agentpack", "archives")) {
					t.Errorf("unexpected path %q", dir)
				}
			},
		},
		{
			name:    "returns error when home dir lookup fails",
			wantErr: "home dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Not parallel: mutates archivesDirHome package-level var.

			var restore func()

			if tt.homeDir != "" {
				restore = install.SetArchivesDirHome(func() (string, error) { return tt.homeDir, nil })
			} else {
				restore = install.SetArchivesDirHome(func() (string, error) {
					return "", errors.New("simulated home dir failure")
				})
			}

			defer restore()

			dir, err := install.ArchivesDir()

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

			if tt.check != nil {
				tt.check(t, dir)
			}
		})
	}
}
