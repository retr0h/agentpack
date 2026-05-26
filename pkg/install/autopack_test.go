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
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	gr, err := gzip.NewReader(f)
	require.NoError(t, err)
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)
	var names []string

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
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
		require.NoError(t, os.MkdirAll(skillDir, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(skillDir, "SKILL.md"),
			[]byte("# "+s+"\n"),
			0o644,
		))
	}

	return dir
}

// makeRepoWithContent creates a temp directory with a mix of content and noise.
func makeRepoWithContent(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Recognized content.
	skillDir := filepath.Join(dir, "skills", "my-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# skill\n"), 0o644),
	)

	cmdDir := filepath.Join(dir, "commands")
	require.NoError(t, os.MkdirAll(cmdDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cmdDir, "scan.md"), []byte("# scan\n"), 0o644))

	// Noise that should never appear in the archive.
	for _, noise := range []string{".github", ".git", "tools", "scripts"} {
		noiseDir := filepath.Join(dir, noise)
		require.NoError(t, os.MkdirAll(noiseDir, 0o755))
		require.NoError(
			t,
			os.WriteFile(filepath.Join(noiseDir, "file.txt"), []byte("noise\n"), 0o644),
		)
	}

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("readme\n"), 0o644))

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
		customCtx    context.Context
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
					assert.True(t, slices.Contains(entries, want))
				}

				// Must NOT contain noise dirs.
				for _, e := range entries {
					for _, bad := range []string{".github", ".git/", "tools/", "scripts/", "README.md"} {
						assert.False(t, strings.HasPrefix(e, bad) || e == bad)
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
					assert.True(t, slices.Contains(entries, want))
				}

				// deploy must be absent.
				for _, e := range entries {
					assert.False(t, strings.HasPrefix(e, "skills/deploy/"))
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
					assert.True(t, slices.Contains(entries, want))
				}
			},
		},
		{
			name: "skills dir with flat files (no subdirs) is packaged whole",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				skillsDir := filepath.Join(dir, "skills")
				require.NoError(t, os.MkdirAll(skillsDir, 0o755))
				require.NoError(
					t,
					os.WriteFile(filepath.Join(skillsDir, "flat.md"), []byte("# flat\n"), 0o644),
				)
				return dir
			},
			sha: "flat1234",
			checkEntries: func(t *testing.T, entries []string) {
				t.Helper()
				assert.True(t, slices.Contains(entries, "skills/flat.md"))
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
		{
			name: "returns error when WalkDir encounters permission denied in skills dir",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				skillDir := filepath.Join(dir, "skills", "my-skill")

				require.NoError(t, os.MkdirAll(skillDir, 0o755))

				locked := filepath.Join(skillDir, "locked")
				require.NoError(t, os.MkdirAll(locked, 0o000))

				t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

				return dir
			},
			sha:     "abc",
			wantErr: "walk skills",
		},
		{
			name: "returns error when context cancelled inside content dirs loop",
			setup: func(t *testing.T) string {
				t.Helper()
				return makeSkillRepo(t, []string{"review"})
			},
			sha:       "abc",
			customCtx: newCancelAfterN(1),
			wantErr:   "context canceled",
		},
		{
			name: "returns error when context cancelled inside roots loop",
			setup: func(t *testing.T) string {
				t.Helper()
				return makeSkillRepo(t, []string{"review"})
			},
			sha:       "abc",
			customCtx: newCancelAfterN(2),
			wantErr:   "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setup(t)

			var ctx context.Context
			var cancel context.CancelFunc

			switch {
			case tt.customCtx != nil:
				ctx = tt.customCtx
				cancel = func() {}
			case tt.cancelCtx:
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			default:
				ctx = context.Background()
				cancel = func() {}
			}

			defer cancel()

			archivePath, err := install.AutoPackage(
				ctx,
				dir,
				"test-pkg",
				tt.sha,
				tt.skillFilter,
				tt.agentFilter,
			)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			defer func() { _ = os.Remove(archivePath) }()

			assert.NotEmpty(t, archivePath)

			_, statErr := os.Stat(archivePath)
			require.NoError(t, statErr)

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
				require.NoError(t, os.MkdirAll(skillsDir, 0o755))
				require.NoError(
					t,
					os.WriteFile(filepath.Join(skillsDir, "flat.md"), []byte("x"), 0o644),
				)
				return dir
			},
			filter:    []string{"anything"},
			wantNames: []string{"skills"},
		},
		{
			name: "returns content dir itself when ReadDir fails (unreadable dir)",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				skillsDir := filepath.Join(dir, "skills")

				require.NoError(t, os.MkdirAll(skillsDir, 0o000))

				t.Cleanup(func() { _ = os.Chmod(skillsDir, 0o755) })

				return dir
			},
			filter:    nil,
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
				assert.True(t, found)
			}

			// Verify missing names do not appear.
			for _, missing := range tt.wantMissing {
				for _, root := range roots {
					assert.NotEqual(t, missing, filepath.Base(root))
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
				require.NoError(t, os.WriteFile(src, []byte("agentpack data"), 0o644))
				storeDir := filepath.Join(dir, "archives")
				require.NoError(t, os.MkdirAll(storeDir, 0o700))
				return src, storeDir
			},
			pkgName: "my-plugin",
			sha:     "abc1234567890abcdef",
			check: func(t *testing.T, dstPath string) {
				t.Helper()
				data, err := os.ReadFile(dstPath)
				require.NoError(t, err)
				assert.Equal(t, "agentpack data", string(data))
				base := filepath.Base(dstPath)
				assert.True(
					t,
					strings.HasPrefix(base, "my-plugin@") && strings.HasSuffix(base, ".agentpack"),
				)
			},
		},
		{
			name: "returns error when source does not exist",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				storeDir := filepath.Join(dir, "archives")
				require.NoError(t, os.MkdirAll(storeDir, 0o700))
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
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

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
				require.NoError(t, err)
				assert.True(t, info.IsDir())
				assert.True(
					t,
					strings.HasSuffix(dir, filepath.Join(".config", "agentpack", "archives")),
				)
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
				restore = install.SetArchivesDirHome(
					func() (string, error) { return tt.homeDir, nil },
				)
			} else {
				restore = install.SetArchivesDirHome(func() (string, error) {
					return "", errors.New("simulated home dir failure")
				})
			}

			defer restore()

			dir, err := install.ArchivesDir()

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, dir)
			}
		})
	}
}
