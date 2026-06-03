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
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avfs/avfs/vfs/osfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	"github.com/retr0h/agentpack/internal/archive"
	"github.com/retr0h/agentpack/internal/metadata"
	"github.com/retr0h/agentpack/internal/target"
	"github.com/retr0h/agentpack/internal/target/mocks"
	"github.com/retr0h/agentpack/internal/testutil"
	"github.com/retr0h/agentpack/pkg/install"
	"github.com/retr0h/agentpack/pkg/registry"
	"github.com/retr0h/agentpack/pkg/safety"
)

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

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
		{
			name: "returns error when context is already cancelled",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				storeDir := filepath.Join(dir, "archives")
				require.NoError(t, os.MkdirAll(storeDir, 0o700))
				return filepath.Join(dir, "unused.agentpack"), storeDir
			},
			pkgName: "ctx-plugin",
			sha:     "abc1234",
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Not parallel: mutates archivesDirFunc package-level var.

			srcPath, storeDir := tt.setup(t)

			// Override archivesDir via the exported test hook.
			restore := install.SetArchivesDir(func() (string, error) { return storeDir, nil })
			defer restore()

			ctx := context.Background()
			if tt.wantErr == "context canceled" {
				cancelCtx, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelCtx
			}

			dstPath, err := install.StoreArchive(ctx, srcPath, tt.pkgName, tt.sha)

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
		{
			name: "returns error when mkdir fails",
			// Point home at a path where a file blocks the .config directory.
			homeDir: func() string {
				tmp := t.TempDir()
				blocker := filepath.Join(tmp, ".config")
				_ = os.WriteFile(blocker, []byte("block"), 0o644)
				return tmp
			}(),
			wantErr: "mkdir archives dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Not parallel: mutates archivesDirHome package-level var.

			var restore func()

			if tt.homeDir != "" {
				capturedHome := tt.homeDir
				restore = install.SetArchivesDirHome(
					func() (string, error) { return capturedHome, nil },
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

// --------------------------------------------------------------------------
// TestBuildContentMap
// --------------------------------------------------------------------------

func TestBuildContentMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) []archive.FileEntry
		wantErr string
		check   func(t *testing.T, m map[string][]byte)
	}{
		{
			name: "returns content for in-memory file entries",
			setup: func(t *testing.T) []archive.FileEntry {
				t.Helper()
				return []archive.FileEntry{
					{ArchivePath: "skills/review/SKILL.md", Content: []byte("# Review\n")},
					{ArchivePath: "commands/deploy.md", Content: []byte("# Deploy\n")},
				}
			},
			check: func(t *testing.T, m map[string][]byte) {
				t.Helper()
				assert.Equal(t, []byte("# Review\n"), m["skills/review/SKILL.md"])
				assert.Equal(t, []byte("# Deploy\n"), m["commands/deploy.md"])
			},
		},
		{
			name: "reads content from disk for file entries with Src set",
			setup: func(t *testing.T) []archive.FileEntry {
				t.Helper()
				dir := t.TempDir()
				skillFile := filepath.Join(dir, "SKILL.md")
				require.NoError(t, os.WriteFile(skillFile, []byte("# On-disk skill\n"), 0o644))
				return []archive.FileEntry{
					{Src: skillFile, ArchivePath: "skills/on-disk/SKILL.md"},
				}
			},
			check: func(t *testing.T, m map[string][]byte) {
				t.Helper()
				assert.Equal(t, []byte("# On-disk skill\n"), m["skills/on-disk/SKILL.md"])
			},
		},
		{
			name: "mixes on-disk and in-memory entries",
			setup: func(t *testing.T) []archive.FileEntry {
				t.Helper()
				dir := t.TempDir()
				onDisk := filepath.Join(dir, "cmd.md")
				require.NoError(t, os.WriteFile(onDisk, []byte("# Cmd\n"), 0o644))
				return []archive.FileEntry{
					{Src: onDisk, ArchivePath: "commands/cmd.md"},
					{ArchivePath: "skills/inline/SKILL.md", Content: []byte("# Inline\n")},
				}
			},
			check: func(t *testing.T, m map[string][]byte) {
				t.Helper()
				assert.Equal(t, []byte("# Cmd\n"), m["commands/cmd.md"])
				assert.Equal(t, []byte("# Inline\n"), m["skills/inline/SKILL.md"])
			},
		},
		{
			name: "returns empty map for empty input",
			setup: func(t *testing.T) []archive.FileEntry {
				t.Helper()
				return []archive.FileEntry{}
			},
			check: func(t *testing.T, m map[string][]byte) {
				t.Helper()
				assert.Empty(t, m)
			},
		},
		{
			name: "returns error when on-disk file cannot be read",
			setup: func(t *testing.T) []archive.FileEntry {
				t.Helper()
				return []archive.FileEntry{
					{Src: "/nonexistent/path/SKILL.md", ArchivePath: "skills/x/SKILL.md"},
				}
			},
			wantErr: "read",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			files := tt.setup(t)
			m, err := install.BuildContentMap(files)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, m)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestWalkContentDir
// --------------------------------------------------------------------------

func TestWalkContentDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (cloneDir, root string)
		wantErr string
		check   func(t *testing.T, entries []archive.FileEntry)
	}{
		{
			name: "walks directory and returns file entries with relative archive paths",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				cloneDir := t.TempDir()
				skillDir := filepath.Join(cloneDir, "skills", "review")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(skillDir, "SKILL.md"),
					[]byte("# Review"),
					0o644,
				))
				require.NoError(t, os.WriteFile(
					filepath.Join(skillDir, "notes.txt"),
					[]byte("notes"),
					0o644,
				))
				return cloneDir, filepath.Join(cloneDir, "skills", "review")
			},
			check: func(t *testing.T, entries []archive.FileEntry) {
				t.Helper()
				require.Len(t, entries, 2)
				// Archive paths must be relative to cloneDir.
				paths := make([]string, len(entries))
				for i, e := range entries {
					paths[i] = e.ArchivePath
				}
				assert.Contains(t, paths, filepath.Join("skills", "review", "SKILL.md"))
				assert.Contains(t, paths, filepath.Join("skills", "review", "notes.txt"))
				// Src must be the absolute on-disk path.
				for _, e := range entries {
					assert.NotEmpty(t, e.Src)
					assert.True(t, filepath.IsAbs(e.Src))
				}
			},
		},
		{
			name: "returns empty slice for directory with no files",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				cloneDir := t.TempDir()
				emptyDir := filepath.Join(cloneDir, "skills", "empty")
				require.NoError(t, os.MkdirAll(emptyDir, 0o755))
				return cloneDir, emptyDir
			},
			check: func(t *testing.T, entries []archive.FileEntry) {
				t.Helper()
				assert.Empty(t, entries)
			},
		},
		{
			name: "returns error when root directory cannot be walked",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				cloneDir := t.TempDir()
				locked := filepath.Join(cloneDir, "skills")
				require.NoError(t, os.MkdirAll(locked, 0o000))
				t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
				return cloneDir, locked
			},
			wantErr: "permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cloneDir, root := tt.setup(t)
			entries, err := install.WalkContentDir(cloneDir, root)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, entries)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestComputeChecksums
// --------------------------------------------------------------------------

func TestComputeChecksums(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) []archive.FileEntry
		wantErr string
		check   func(t *testing.T, out []byte)
	}{
		{
			name: "computes checksums for in-memory entries",
			setup: func(t *testing.T) []archive.FileEntry {
				t.Helper()
				return []archive.FileEntry{
					{ArchivePath: "skills/intro.md", Content: []byte("# Intro")},
				}
			},
			check: func(t *testing.T, out []byte) {
				t.Helper()
				assert.Contains(t, string(out), "skills/intro.md")
				// sha256sum format: <64-hex>  <path>\n
				lines := strings.Split(strings.TrimSpace(string(out)), "\n")
				require.Len(t, lines, 1)
				parts := strings.SplitN(lines[0], "  ", 2)
				require.Len(t, parts, 2)
				assert.Len(t, parts[0], 64)
			},
		},
		{
			name: "computes checksums for on-disk entries",
			setup: func(t *testing.T) []archive.FileEntry {
				t.Helper()
				dir := t.TempDir()
				f := filepath.Join(dir, "SKILL.md")
				require.NoError(t, os.WriteFile(f, []byte("# Skill"), 0o644))
				return []archive.FileEntry{
					{Src: f, ArchivePath: "skills/skill/SKILL.md"},
				}
			},
			check: func(t *testing.T, out []byte) {
				t.Helper()
				assert.Contains(t, string(out), "skills/skill/SKILL.md")
				lines := strings.Split(strings.TrimSpace(string(out)), "\n")
				require.Len(t, lines, 1)
				parts := strings.SplitN(lines[0], "  ", 2)
				require.Len(t, parts, 2)
				assert.Len(t, parts[0], 64)
			},
		},
		{
			name: "returns empty output for empty input",
			setup: func(t *testing.T) []archive.FileEntry {
				t.Helper()
				return []archive.FileEntry{}
			},
			check: func(t *testing.T, out []byte) {
				t.Helper()
				assert.Empty(t, out)
			},
		},
		{
			name: "returns error when on-disk file cannot be read",
			setup: func(t *testing.T) []archive.FileEntry {
				t.Helper()
				return []archive.FileEntry{
					{Src: "/nonexistent/SKILL.md", ArchivePath: "skills/x/SKILL.md"},
				}
			},
			wantErr: "read",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			files := tt.setup(t)
			out, err := install.ComputeChecksums(files)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestAutoPackageWithVersion
// --------------------------------------------------------------------------

// extractMetadataFromArchive opens the .agentpack tarball at path and returns
// the parsed metadata.Metadata. It prefers .agentpack/metadata.yaml and falls
// back to .agentpack/metadata.json for backward compatibility.
func extractMetadataFromArchive(t *testing.T, archivePath string) *metadata.Metadata {
	t.Helper()

	f, err := os.Open(archivePath)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	gr, err := gzip.NewReader(f)
	require.NoError(t, err)
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)

	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}

		if strings.HasSuffix(hdr.Name, "metadata.yaml") {
			data, readErr := io.ReadAll(tr)
			require.NoError(t, readErr)

			var meta metadata.Metadata
			require.NoError(t, yaml.Unmarshal(data, &meta))

			return &meta
		}

		if strings.HasSuffix(hdr.Name, "metadata.json") {
			var meta metadata.Metadata
			require.NoError(t, json.NewDecoder(tr).Decode(&meta))

			return &meta
		}
	}

	require.FailNow(t, "")

	return nil
}

func TestAutoPackageWithVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		setup        func(t *testing.T) string // returns cloneDir
		skillFilter  []string
		agentFilter  []string
		wantErr      string
		checkArchive func(t *testing.T, archivePath string)
	}{
		{
			name: "produces archive with content field in metadata",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				skillDir := filepath.Join(dir, "skills", "review")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(skillDir, "SKILL.md"),
					[]byte("# Review skill\n"),
					0o644,
				))
				return dir
			},
			checkArchive: func(t *testing.T, archivePath string) {
				t.Helper()
				meta := extractMetadataFromArchive(t, archivePath)
				require.NotNil(t, meta.Content)
				assert.Contains(t, meta.Content.Safe, filepath.Join("skills", "review", "SKILL.md"))
			},
		},
		{
			name: "produces valid archive with no content dirs present",
			setup: func(t *testing.T) string {
				t.Helper()
				// A cloneDir with no recognized content directories.
				return t.TempDir()
			},
			checkArchive: func(t *testing.T, archivePath string) {
				t.Helper()
				meta := extractMetadataFromArchive(t, archivePath)
				require.NotNil(t, meta.Content)
				assert.Empty(t, meta.Content.Safe)
				assert.Empty(t, meta.Content.Executable)
			},
		},
		{
			name: "returns error when a content file is binary",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				cmdDir := filepath.Join(dir, "commands")
				require.NoError(t, os.MkdirAll(cmdDir, 0o755))
				// ELF magic bytes — always classified as binary.
				elfBytes := []byte{0x7f, 'E', 'L', 'F', 0x00, 0x00, 0x00, 0x00}
				require.NoError(t, os.WriteFile(
					filepath.Join(cmdDir, "elf-binary"),
					elfBytes,
					0o755,
				))
				return dir
			},
			wantErr: "binary file detected",
		},
		{
			name: "returns error when context is already cancelled",
			setup: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: "context canceled",
		},
		{
			name: "returns error when context cancelled inside content dirs loop",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				skillDir := filepath.Join(dir, "skills", "review")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(skillDir, "SKILL.md"),
					[]byte("# review\n"),
					0o644,
				))
				return dir
			},
			wantErr: "context canceled",
		},
		{
			name: "returns error when context cancelled inside walkRoots inner loop",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				skillDir := filepath.Join(dir, "skills", "review")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(skillDir, "SKILL.md"),
					[]byte("# review\n"),
					0o644,
				))
				return dir
			},
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cloneDir := tt.setup(t)

			var ctx context.Context
			switch tt.name {
			case "returns error when context is already cancelled":
				cancelCtx, cancel := context.WithCancel(context.Background())
				cancel()
				ctx = cancelCtx
			case "returns error when context cancelled inside content dirs loop":
				ctx = testutil.NewCancelAfterN(1)
			case "returns error when context cancelled inside walkRoots inner loop":
				ctx = testutil.NewCancelAfterN(2)
			default:
				ctx = context.Background()
			}

			archivePath, err := install.AutoPackageWithVersion(
				ctx,
				cloneDir,
				"test-plugin",
				"abc1234567890",
				"1.0.0",
				tt.skillFilter,
				tt.agentFilter,
			)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotEmpty(t, archivePath)

			t.Cleanup(func() { _ = os.Remove(archivePath) })

			if tt.checkArchive != nil {
				tt.checkArchive(t, archivePath)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestAutoPackageGeneratesEntries
// --------------------------------------------------------------------------

func TestAutoPackageGeneratesEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		skillFilter []string
		agentFilter []string
		check       func(t *testing.T, meta *metadata.Metadata)
	}{
		{
			name: "skills and commands produce typed entries",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()

				k8sDir := filepath.Join(dir, "skills", "k8s")
				require.NoError(t, os.MkdirAll(k8sDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(k8sDir, "SKILL.md"),
					[]byte("# Kubernetes\n"),
					0o644,
				))

				cmdDir := filepath.Join(dir, "commands")
				require.NoError(t, os.MkdirAll(cmdDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(cmdDir, "scan.md"),
					[]byte("# Scan\n"),
					0o644,
				))

				return dir
			},
			check: func(t *testing.T, meta *metadata.Metadata) {
				t.Helper()
				require.Len(t, meta.Entries, 2)

				entryMap := make(map[string]string, len(meta.Entries))
				for _, e := range meta.Entries {
					entryMap[e.Name] = e.Type
				}

				assert.Equal(t, "skill", entryMap["k8s"])
				assert.Equal(t, "command", entryMap["commands"])
			},
		},
		{
			name: "skill filter restricts entries",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()

				for _, name := range []string{"k8s", "react"} {
					skillDir := filepath.Join(dir, "skills", name)
					require.NoError(t, os.MkdirAll(skillDir, 0o755))
					require.NoError(t, os.WriteFile(
						filepath.Join(skillDir, "SKILL.md"),
						[]byte("# "+name+"\n"),
						0o644,
					))
				}

				return dir
			},
			skillFilter: []string{"k8s"},
			check: func(t *testing.T, meta *metadata.Metadata) {
				t.Helper()
				require.Len(t, meta.Entries, 1)
				assert.Equal(t, "k8s", meta.Entries[0].Name)
				assert.Equal(t, "skill", meta.Entries[0].Type)
			},
		},
		{
			name: "agent filter restricts to agents only",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()

				for _, name := range []string{"reviewer", "planner"} {
					agentDir := filepath.Join(dir, "agents", name)
					require.NoError(t, os.MkdirAll(agentDir, 0o755))
					require.NoError(t, os.WriteFile(
						filepath.Join(agentDir, "AGENT.md"),
						[]byte("# "+name+"\n"),
						0o644,
					))
				}

				return dir
			},
			agentFilter: []string{"reviewer"},
			check: func(t *testing.T, meta *metadata.Metadata) {
				t.Helper()
				require.Len(t, meta.Entries, 1)
				assert.Equal(t, "reviewer", meta.Entries[0].Name)
				assert.Equal(t, "agent", meta.Entries[0].Type)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cloneDir := tt.setup(t)

			archivePath, err := install.AutoPackageWithVersion(
				context.Background(),
				cloneDir,
				"test-plugin",
				"abc1234567890",
				"1.0.0",
				tt.skillFilter,
				tt.agentFilter,
			)
			require.NoError(t, err)
			require.NotEmpty(t, archivePath)

			t.Cleanup(func() { _ = os.Remove(archivePath) })

			meta := extractMetadataFromArchive(t, archivePath)
			require.NotNil(t, meta)

			tt.check(t, meta)
		})
	}
}

// --------------------------------------------------------------------------
// TestContentCheckCallback
// --------------------------------------------------------------------------

// buildArchiveWithContentClassification creates a well-formed .agentpack archive
// that includes a metadata.json with a non-nil Content field.
func buildArchiveWithContentClassification(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()
	outPath := filepath.Join(dir, "classified.agentpack")

	content := []byte("# Skill")
	filePath := "skills/intro.md"

	meta := metadata.Metadata{
		Name:         "classified-plugin",
		Version:      "1.0.0",
		GitCommitSHA: "abc1234567890",
		Content: &safety.Classification{
			Safe: []string{filePath},
		},
	}

	metaJSON, err := json.Marshal(meta)
	require.NoError(t, err)

	checksumsContent := buildArchiveChecksums(t, content, filePath, metaJSON)

	require.NoError(t, archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{ArchivePath: filePath, Content: content},
		{ArchivePath: ".agentpack/metadata.json", Content: metaJSON},
		{ArchivePath: ".agentpack/checksums.txt", Content: []byte(checksumsContent)},
	}))

	return outPath
}

// buildArchiveChecksums produces the sha256sum-format content for two entries.
func buildArchiveChecksums(
	t *testing.T,
	fileContent []byte,
	filePath string,
	metaJSON []byte,
) string {
	t.Helper()
	return strings.Join([]string{
		sha256Hex(fileContent) + "  " + filePath,
		sha256Hex(metaJSON) + "  .agentpack/metadata.json",
		"",
	}, "\n")
}

func TestContentCheckCallback(t *testing.T) {
	// NOTE: not parallel — subtests mutate the shared registrySave package-level var.

	tests := []struct {
		name         string
		contentCheck func(*safety.Classification) error
		wantErr      string
		checkResult  func(t *testing.T, r *install.Result)
	}{
		{
			name:         "nil ContentCheck proceeds unconditionally",
			contentCheck: nil,
			checkResult: func(t *testing.T, r *install.Result) {
				t.Helper()
				require.NotNil(t, r)
				require.NotNil(t, r.ContentClassification)
				assert.Contains(t, r.ContentClassification.Safe, "skills/intro.md")
			},
		},
		{
			name: "ContentCheck returning nil allows install to proceed",
			contentCheck: func(_ *safety.Classification) error {
				return nil
			},
			checkResult: func(t *testing.T, r *install.Result) {
				t.Helper()
				require.NotNil(t, r)
				require.NotNil(t, r.ContentClassification)
			},
		},
		{
			name: "ContentCheck returning error aborts install",
			contentCheck: func(_ *safety.Classification) error {
				return errors.New("content policy violation")
			},
			wantErr: "content policy violation",
		},
		{
			name: "ContentCheck receives correct classification data",
			contentCheck: func(c *safety.Classification) error {
				if !strings.Contains(strings.Join(c.Safe, ","), "skills/intro.md") {
					return errors.New("expected skills/intro.md in safe list")
				}
				return nil
			},
			checkResult: func(t *testing.T, r *install.Result) {
				t.Helper()
				require.NotNil(t, r)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Not parallel: mutates registrySave package-level var.

			restoreSave := install.SetRegistrySave(
				func(_ *registry.PackageManifest) error { return nil },
			)
			defer restoreSave()

			archivePath := buildArchiveWithContentClassification(t)

			ctrl := gomock.NewController(t)
			mockTarget := mocks.NewMockTarget(ctrl)
			mockTarget.EXPECT().Name().Return("test-target").AnyTimes()
			mockTarget.EXPECT().DisplayName().Return("Test Target").AnyTimes()
			mockTarget.EXPECT().
				SupportedTypes().
				Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
				AnyTimes()
			mockTarget.EXPECT().Install(gomock.Any(), gomock.Any()).Return([]target.InstalledFile{
				{Path: "skills/intro.md", SHA256: "dummy"},
			}, nil).AnyTimes()

			r, err := install.NewWithTargets([]target.Target{mockTarget}).
				Run(context.Background(), install.Options{
					Source:       archivePath,
					ContentCheck: tt.contentCheck,
				})

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.checkResult != nil {
				tt.checkResult(t, r)
			}
		})
	}
}
