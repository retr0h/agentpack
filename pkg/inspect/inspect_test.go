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

package inspect_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/avfs/avfs/vfs/osfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/archive"
	"github.com/retr0h/agentpack/internal/testutil"
	"github.com/retr0h/agentpack/pkg/inspect"
)

// sha256Hex returns the hex-encoded SHA256 of data.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// buildValidArchive creates a minimal .agentpack archive with one content file,
// a metadata.json, and a checksums.txt. Returns the archive path.
func buildValidArchive(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()

	skillContent := []byte("# Intro skill")
	skillPath := "skills/intro.md"

	metaContent, err := json.Marshal(map[string]string{
		"name":           "my-plugin",
		"version":        "v1.0.0",
		"gitCommitSHA":   "abc1234def5678",
		"buildTimestamp": "2026-05-20T10:00:00Z",
	})
	require.NoError(t, err)

	metaPath := ".agentpack/metadata.json"
	checksumPath := ".agentpack/checksums.txt"

	skillHash := sha256Hex(skillContent)
	metaHash := sha256Hex(metaContent)

	checksumContent := fmt.Sprintf("%s  %s\n%s  %s\n", skillHash, skillPath, metaHash, metaPath)

	outPath := filepath.Join(dir, "my-plugin-v1.0.0.agentpack")
	require.NoError(t, archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{ArchivePath: skillPath, Content: skillContent},
		{ArchivePath: metaPath, Content: metaContent},
		{ArchivePath: checksumPath, Content: []byte(checksumContent)},
	}))

	return outPath
}

// --------------------------------------------------------------------------
// Run
// --------------------------------------------------------------------------

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		archivePath func(t *testing.T) string
		ctx         func() context.Context
		injectFuncs func(t *testing.T) // if set, swap package vars (not parallel-safe)
		noParallel  bool               // if true, skip t.Parallel()
		wantErr     string
		checkResult func(t *testing.T, r *inspect.Result)
	}{
		{
			name:        "happy path returns populated result",
			archivePath: buildValidArchive,
			ctx:         func() context.Context { return context.Background() },
			checkResult: func(t *testing.T, r *inspect.Result) {
				t.Helper()
				require.NotNil(t, r)
				assert.Equal(t, "my-plugin", r.Name)
				assert.Equal(t, "v1.0.0", r.Version)
				assert.Equal(t, "2026-05-20T10:00:00Z", r.Built)
				assert.Equal(t, "abc1234def5678", r.SHA)
				assert.NotEmpty(t, r.Files)
				assert.Positive(t, r.Total)

				paths := make([]string, 0, len(r.Files))
				for _, f := range r.Files {
					paths = append(paths, f.Path)
				}
				assert.Contains(t, paths, "skills/intro.md")
				assert.Contains(t, paths, ".agentpack/metadata.json")
				assert.Contains(t, paths, ".agentpack/checksums.txt")
			},
		},
		{
			name:        "verified flag set for matching checksums",
			archivePath: buildValidArchive,
			ctx:         func() context.Context { return context.Background() },
			checkResult: func(t *testing.T, r *inspect.Result) {
				t.Helper()
				require.NotNil(t, r)
				for _, f := range r.Files {
					if f.Path == "skills/intro.md" || f.Path == ".agentpack/metadata.json" {
						assert.True(t, f.Verified)
					}
				}
			},
		},
		{
			name: "file not found returns error",
			archivePath: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "nonexistent.agentpack")
			},
			ctx:     func() context.Context { return context.Background() },
			wantErr: "extract",
		},
		{
			name:        "cancelled context returns error",
			archivePath: buildValidArchive,
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantErr: "context canceled",
		},
		{
			name: "missing metadata.json returns error",
			archivePath: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				vfs := osfs.NewWithNoIdm()
				outPath := filepath.Join(dir, "no-meta.agentpack")
				require.NoError(
					t,
					archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
						{ArchivePath: "skills/intro.md", Content: []byte("content")},
						{
							ArchivePath: ".agentpack/checksums.txt",
							Content:     []byte("hash  skills/intro.md\n"),
						},
					}),
				)
				return outPath
			},
			ctx:     func() context.Context { return context.Background() },
			wantErr: "read metadata.json",
		},
		{
			name: "missing checksums.txt returns error",
			archivePath: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				vfs := osfs.NewWithNoIdm()
				metaContent, err := json.Marshal(map[string]string{
					"name":    "x",
					"version": "1.0.0",
				})
				require.NoError(t, err)
				outPath := filepath.Join(dir, "no-checksums.agentpack")
				require.NoError(
					t,
					archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
						{ArchivePath: ".agentpack/metadata.json", Content: metaContent},
					}),
				)
				return outPath
			},
			ctx:     func() context.Context { return context.Background() },
			wantErr: "read checksums.txt",
		},
		{
			name: "invalid metadata.json returns parse error",
			archivePath: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				vfs := osfs.NewWithNoIdm()
				outPath := filepath.Join(dir, "bad-meta.agentpack")
				require.NoError(
					t,
					archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
						{ArchivePath: ".agentpack/metadata.json", Content: []byte("not-json")},
						{ArchivePath: ".agentpack/checksums.txt", Content: []byte("hash  file\n")},
					}),
				)
				return outPath
			},
			ctx:     func() context.Context { return context.Background() },
			wantErr: "parse metadata.json",
		},
		{
			name:       "osMkdirTemp failure returns error",
			noParallel: true,
			archivePath: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "unused.agentpack")
			},
			ctx: func() context.Context { return context.Background() },
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := inspect.SetOsMkdirTemp(inspect.MkdirTempAlwaysFails)
				t.Cleanup(restore)
			},
			wantErr: "create temp dir",
		},
		{
			// Call 1 (line 96) passes; calls 2-7 inside Extract; call 8 (line 110)
			// fires after Extract returns → triggers the post-extract ctx check.
			name:        "cancelled context returns error after extract",
			archivePath: buildValidArchive,
			ctx:         func() context.Context { return testutil.NewCancelAfterN(7) },
			wantErr:     "context canceled",
		},
		{
			// Call 1 (line 96) + 6 (Extract) + 1 (line 110) = 8 pass;
			// call 9 fires at line 125 after metadata is parsed.
			name:        "cancelled context returns error after parsing metadata",
			archivePath: buildValidArchive,
			ctx:         func() context.Context { return testutil.NewCancelAfterN(8) },
			wantErr:     "context canceled",
		},
		{
			// Calls 1-9 pass; call 10 fires at line 141 after checksums are read.
			name:        "cancelled context returns error after reading checksums",
			archivePath: buildValidArchive,
			ctx:         func() context.Context { return testutil.NewCancelAfterN(9) },
			wantErr:     "context canceled",
		},
		{
			// Calls 1-10 pass; call 11 fires inside the WalkDir callback (line 161)
			// when processing the first content file.
			name:        "cancelled context returns error inside walk callback",
			archivePath: buildValidArchive,
			ctx:         func() context.Context { return testutil.NewCancelAfterN(10) },
			wantErr:     "context canceled",
		},
		{
			// Calls 1-11 pass; call 12 fires at line 194 after the walk completes.
			name:        "cancelled context returns error after walk",
			archivePath: buildValidArchive,
			ctx:         func() context.Context { return testutil.NewCancelAfterN(11) },
			wantErr:     "context canceled",
		},
		{
			// content field is populated when archive metadata carries a safety
			// classification — verifies the nil-omitempty branch is exercised.
			name: "content classification propagated from metadata",
			archivePath: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				vfs := osfs.NewWithNoIdm()
				type classification struct {
					Level string `json:"level"`
				}
				type meta struct {
					Name           string          `json:"name"`
					Version        string          `json:"version"`
					GitCommitSHA   string          `json:"gitCommitSHA"`
					BuildTimestamp string          `json:"buildTimestamp"`
					Content        *classification `json:"content,omitempty"`
				}
				metaContent, err := json.Marshal(meta{
					Name:           "classified-plugin",
					Version:        "v2.0.0",
					GitCommitSHA:   "deadbeef",
					BuildTimestamp: "2026-05-20T10:00:00Z",
					Content:        &classification{Level: "safe"},
				})
				require.NoError(t, err)
				checksumContent := fmt.Sprintf(
					"%s  .agentpack/metadata.json\n",
					sha256Hex(metaContent),
				)
				outPath := filepath.Join(dir, "classified.agentpack")
				require.NoError(
					t,
					archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
						{ArchivePath: ".agentpack/metadata.json", Content: metaContent},
						{ArchivePath: ".agentpack/checksums.txt", Content: []byte(checksumContent)},
					}),
				)
				return outPath
			},
			ctx: func() context.Context { return context.Background() },
			checkResult: func(t *testing.T, r *inspect.Result) {
				t.Helper()
				require.NotNil(t, r)
				assert.Equal(t, "classified-plugin", r.Name)
				assert.NotNil(t, r.Content)
			},
		},
		{
			// Pre-populate the temp dir with a subdirectory the test process cannot
			// read (chmod 000). WalkDir calls the callback with a non-nil walkErr
			// when it fails to descend into that directory, covering the
			// "if walkErr != nil { return walkErr }" branch and the subsequent
			// "walk archive" error return.
			name:        "unreadable directory in extracted temp causes walk error",
			noParallel:  true,
			archivePath: buildValidArchive,
			ctx:         func() context.Context { return context.Background() },
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := inspect.SetOsMkdirTemp(func(dir, pattern string) (string, error) {
					tmp, err := os.MkdirTemp(dir, pattern)
					if err != nil {
						return "", err
					}
					// Name starts with "aaaa" so it sorts before "skills/" and
					// is encountered by WalkDir before any legitimate content.
					restricted := filepath.Join(tmp, "aaaa-restricted")
					if mkErr := os.Mkdir(restricted, 0); mkErr != nil {
						return "", mkErr
					}
					// Restore permissions so the deferred RemoveAll inside Run can
					// clean up without errors.
					t.Cleanup(func() { _ = os.Chmod(restricted, 0o700) })
					return tmp, nil
				})
				t.Cleanup(restore)
			},
			wantErr: "walk archive",
		},
		{
			// Pre-populate the temp dir with a regular file the test process cannot
			// open (chmod 000). WalkDir visits it as a content file; computeFileHash
			// fails on os.Open, covering both the "return err" branch inside the
			// walk callback and the "open" error return inside computeFileHash.
			name:        "unreadable file in extracted temp causes hash error",
			noParallel:  true,
			archivePath: buildValidArchive,
			ctx:         func() context.Context { return context.Background() },
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := inspect.SetOsMkdirTemp(func(dir, pattern string) (string, error) {
					tmp, err := os.MkdirTemp(dir, pattern)
					if err != nil {
						return "", err
					}
					// Name starts with "aaaa" so it sorts before "skills/" and is
					// encountered by WalkDir before any legitimate content file.
					restricted := filepath.Join(tmp, "aaaa-restricted.txt")
					if wErr := os.WriteFile(restricted, []byte("data"), 0); wErr != nil {
						return "", wErr
					}
					// Restore read permission so the deferred RemoveAll inside Run
					// can clean up the file.
					t.Cleanup(func() { _ = os.Chmod(restricted, 0o600) })
					return tmp, nil
				})
				t.Cleanup(restore)
			},
			wantErr: "walk archive",
		},
		{
			// Inject a failing osReadFile to cover the error branch that fires when
			// os.ReadFile cannot read .agentpack/checksums.txt during the meta-file
			// append loop (the else branch for name != "metadata.json").
			name:        "osReadFile failure for checksums.txt returns error",
			noParallel:  true,
			archivePath: buildValidArchive,
			ctx:         func() context.Context { return context.Background() },
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := inspect.SetOsReadFile(inspect.ReadFileAlwaysFails)
				t.Cleanup(restore)
			},
			wantErr: "read .agentpack/checksums.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.noParallel {
				t.Parallel()
			}

			if tt.injectFuncs != nil {
				tt.injectFuncs(t)
			}

			archivePath := tt.archivePath(t)
			ctx := tt.ctx()

			result, err := inspect.New().Run(ctx, inspect.Options{Path: archivePath})

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.checkResult != nil {
				tt.checkResult(t, result)
			}
		})
	}
}

func TestIsArchiveFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "agentpack extension",
			path: "plugin.agentpack",
			want: true,
		},
		{
			name: "nested path with extension",
			path: "dist/org/my-plugin.agentpack",
			want: true,
		},
		{
			name: "different extension",
			path: "plugin.tar.gz",
			want: false,
		},
		{
			name: "no extension",
			path: "plugin",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, inspect.IsArchiveFile(tt.path))
		})
	}
}
