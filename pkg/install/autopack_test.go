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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avfs/avfs/vfs/osfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/retr0h/agentpack/internal/archive"
	"github.com/retr0h/agentpack/internal/metadata"
	"github.com/retr0h/agentpack/internal/safety"
	"github.com/retr0h/agentpack/pkg/install"
	"github.com/retr0h/agentpack/pkg/registry"
	"github.com/retr0h/agentpack/pkg/target"
	"github.com/retr0h/agentpack/pkg/target/mocks"
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
// the parsed metadata.Metadata from .agentpack/metadata.json.
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

		if strings.HasSuffix(hdr.Name, "metadata.json") {
			var meta metadata.Metadata
			require.NoError(t, json.NewDecoder(tr).Decode(&meta))
			return &meta
		}
	}

	t.Fatal("metadata.json not found in archive")
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cloneDir := tt.setup(t)

			ctx := context.Background()
			if tt.name == "returns error when context is already cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
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
			mockTarget.EXPECT().Install(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

			r, err := install.New().Run(context.Background(), install.Options{
				Source:       archivePath,
				Targets:      []target.Target{mockTarget},
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
