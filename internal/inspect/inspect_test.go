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
	"path/filepath"
	"testing"

	"github.com/avfs/avfs/vfs/osfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/internal/archive"
	"github.com/retr0h/agentpack/internal/inspect"
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

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
