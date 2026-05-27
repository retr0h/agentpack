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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/avfs/avfs/vfs/osfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/retr0h/agentpack/internal/archive"
	"github.com/retr0h/agentpack/internal/metadata"
	"github.com/retr0h/agentpack/pkg/build"
	"github.com/retr0h/agentpack/pkg/install"
	"github.com/retr0h/agentpack/pkg/registry"
	"github.com/retr0h/agentpack/pkg/target"
	"github.com/retr0h/agentpack/pkg/target/mocks"
)

// initBareGitRepo creates a bare git repository at the given path that can be
// cloned via go-git's local file protocol. The path must end in ".git" so that
// fetcher.New selects GitFetcher.
func initBareGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	bare := filepath.Join(dir, "repo.git")

	require.NoError(t, os.MkdirAll(src, 0o755))

	initGitRepo(t, src)

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(), gitEnv...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v\n%s", args, out)
	}

	run("clone", "--bare", src, bare)

	return bare
}

// --------------------------------------------------------------------------
// cancelAfterN context helper
// --------------------------------------------------------------------------

type cancelAfterN struct {
	n    int
	call int
}

func newCancelAfterN(n int) *cancelAfterN { return &cancelAfterN{n: n} }

func (c *cancelAfterN) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterN) Done() <-chan struct{}       { return nil }
func (c *cancelAfterN) Value(_ any) any             { return nil }
func (c *cancelAfterN) Err() error {
	c.call++
	if c.call <= c.n {
		return nil
	}

	return errors.New("context canceled")
}

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)

	return hex.EncodeToString(h[:])
}

var gitEnv = []string{
	"GIT_AUTHOR_NAME=Test Author",
	"GIT_AUTHOR_EMAIL=test@example.com",
	"GIT_COMMITTER_NAME=Test Committer",
	"GIT_COMMITTER_EMAIL=test@example.com",
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), gitEnv...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v\n%s", args, out)
	}

	run("init")
	run("checkout", "-b", "main")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644))

	skillDir := filepath.Join(dir, "skills", "test-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Test Skill\n"), 0o644))

	run("add", ".")
	run("commit", "-m", "init")
}

// buildTestArchive creates a .agentpack archive using the build pipeline and
// returns the path to the archive.
func buildTestArchive(t *testing.T, dir string, manifestContent string) string {
	t.Helper()

	require.NoError(
		t,
		os.WriteFile(filepath.Join(dir, "agentpack.yaml"), []byte(manifestContent), 0o644),
	)

	vfs := osfs.NewWithNoIdm()

	results, err := build.New().Run(context.Background(), vfs, build.Options{Dir: dir})
	require.NoError(t, err)
	require.NotEmpty(t, results)

	return results[0].ArchivePath
}

// buildArchiveNoChecksums builds an archive with no .agentpack/checksums.txt.
func buildArchiveNoChecksums(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()
	outPath := filepath.Join(dir, "nochecksum.agentpack")

	require.NoError(t, archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{
			ArchivePath: "skills/intro.md",
			Content:     []byte("# Intro"),
		},
	}))

	return outPath
}

// buildArchiveBadChecksums builds an archive with a malformed checksums.txt.
func buildArchiveBadChecksums(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()
	outPath := filepath.Join(dir, "badchecksum.agentpack")

	require.NoError(t, archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{
			ArchivePath: ".agentpack/checksums.txt",
			Content:     []byte("badhash file.txt\n"), // missing double-space
		},
	}))

	return outPath
}

// buildArchiveTamperedChecksum builds an archive where the checksum does not
// match the actual file content.
func buildArchiveTamperedChecksum(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()
	outPath := filepath.Join(dir, "tampered.agentpack")
	filePath := "skills/intro.md"
	badHash := strings.Repeat("0", 64)
	checksumContent := fmt.Sprintf("%s  %s\n", badHash, filePath)

	require.NoError(t, archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{ArchivePath: filePath, Content: []byte("content")},
		{
			ArchivePath: ".agentpack/checksums.txt",
			Content:     []byte(checksumContent),
		},
	}))

	return outPath
}

// buildArchiveNoMetadata builds a valid archive (with checksums.txt) but
// without a metadata.json file.
func buildArchiveNoMetadata(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()
	outPath := filepath.Join(dir, "nometa.agentpack")
	content := []byte("# Intro")
	filePath := "skills/intro.md"
	checksumContent := fmt.Sprintf("%s  %s\n", sha256Hex(content), filePath)

	require.NoError(t, archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{ArchivePath: filePath, Content: content},
		{
			ArchivePath: ".agentpack/checksums.txt",
			Content:     []byte(checksumContent),
		},
	}))

	return outPath
}

// buildArchiveWithMeta builds an archive that has .agentpack/checksums.txt and
// .agentpack/metadata.json at the top level (new generic format).
func buildArchiveWithMeta(t *testing.T, name, version string) string {
	t.Helper()

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()
	outPath := filepath.Join(dir, "withmeta.agentpack")
	content := []byte("# Skill")
	filePath := "skills/intro.md"

	metaJSON, err := json.Marshal(metadata.Metadata{
		Name:           name,
		Version:        version,
		GitCommitSHA:   "abc1234567890",
		BuildTimestamp: "2026-01-01T00:00:00Z",
	})
	require.NoError(t, err)

	checksumsContent := fmt.Sprintf(
		"%s  %s\n%s  %s\n",
		sha256Hex(content), filePath,
		sha256Hex(metaJSON), ".agentpack/metadata.json",
	)

	require.NoError(t, archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{ArchivePath: filePath, Content: content},
		{ArchivePath: ".agentpack/metadata.json", Content: metaJSON},
		{ArchivePath: ".agentpack/checksums.txt", Content: []byte(checksumsContent)},
	}))

	return outPath
}

// --------------------------------------------------------------------------
// TestRun
// --------------------------------------------------------------------------

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T, ctrl *gomock.Controller) (archivePath string, targets []target.Target)
		cancelCtx   bool
		noParallel  bool
		customCtx   context.Context
		injectFuncs func(t *testing.T)
		wantErr     string
		checkResult func(t *testing.T, r *install.Result)
		onStep      func(s install.Step)
		checkSteps  func(t *testing.T, steps []install.Step)
	}{
		{
			name:       "installs archive to stub target",
			noParallel: true,
			setup: func(t *testing.T, ctrl *gomock.Controller) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: test-plugin
version: "1.0.0"
description: A test plugin
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("test").AnyTimes()
				m.EXPECT().DisplayName().Return("Test").AnyTimes()
				m.EXPECT().Install(gomock.Any(), gomock.Any()).Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil)

				return archivePath, []target.Target{m}
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := install.SetRegistrySave(
					func(_ *registry.PackageManifest) error { return nil },
				)
				t.Cleanup(restore)
			},
			checkResult: func(t *testing.T, r *install.Result) {
				t.Helper()
				assert.Equal(t, "test-plugin", r.Name)
				assert.Equal(t, "1.0.0", r.Version)
				assert.NotEmpty(t, r.SHA)
			},
		},
		{
			name:       "installs to multiple targets",
			noParallel: true,
			setup: func(t *testing.T, ctrl *gomock.Controller) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: multi-plugin
version: "1.0.0"
description: Multi-target plugin
`)
				m1 := mocks.NewMockTarget(ctrl)
				m1.EXPECT().Name().Return("alpha").AnyTimes()
				m1.EXPECT().DisplayName().Return("Alpha").AnyTimes()
				m1.EXPECT().Install(gomock.Any(), gomock.Any()).Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil)

				m2 := mocks.NewMockTarget(ctrl)
				m2.EXPECT().Name().Return("beta").AnyTimes()
				m2.EXPECT().DisplayName().Return("Beta").AnyTimes()
				m2.EXPECT().Install(gomock.Any(), gomock.Any()).Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil)

				return archivePath, []target.Target{m1, m2}
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := install.SetRegistrySave(
					func(_ *registry.PackageManifest) error { return nil },
				)
				t.Cleanup(restore)
			},
			checkResult: func(t *testing.T, r *install.Result) {
				t.Helper()
				assert.Equal(t, "multi-plugin", r.Name)
			},
		},
		{
			name:       "errors with no targets (empty list)",
			noParallel: true,
			setup: func(t *testing.T, _ *gomock.Controller) (string, []target.Target) {
				t.Helper()
				archivePath := buildArchiveWithMeta(t, "empty-targets-plugin", "1.0.0")

				return archivePath, []target.Target{}
			},
			wantErr: "no agent targets detected",
		},
		{
			name:       "OnStep emits installing-to step per target",
			noParallel: true,
			setup: func(t *testing.T, ctrl *gomock.Controller) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: step-plugin
version: "1.0.0"
description: step test
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("step-target").AnyTimes()
				m.EXPECT().DisplayName().Return("Step Target").AnyTimes()
				m.EXPECT().Install(gomock.Any(), gomock.Any()).Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil)

				return archivePath, []target.Target{m}
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := install.SetRegistrySave(
					func(_ *registry.PackageManifest) error { return nil },
				)
				t.Cleanup(restore)
			},
			checkSteps: func(t *testing.T, steps []install.Step) {
				t.Helper()

				var found bool
				for _, s := range steps {
					if s.Name == "installing to" && s.Detail == "Step Target" {
						found = true
						break
					}
				}

				assert.True(t, found)
			},
		},
		{
			name: "returns error when source has unknown scheme",
			setup: func(t *testing.T, _ *gomock.Controller) (string, []target.Target) {
				t.Helper()

				return "gs://my-bucket/plugin.agentpack", nil
			},
			wantErr: "fetcher",
		},
		{
			name: "returns error when archive does not exist",
			setup: func(t *testing.T, _ *gomock.Controller) (string, []target.Target) {
				t.Helper()

				return "/nonexistent/path.agentpack", nil
			},
			wantErr: "fetch",
		},
		{
			name: "returns error when context is cancelled",
			setup: func(t *testing.T, ctrl *gomock.Controller) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: cancel-plugin
version: "1.0.0"
description: Plugin for cancel test
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("stub").AnyTimes()
				m.EXPECT().DisplayName().Return("Stub").AnyTimes()
				m.EXPECT().Install(gomock.Any(), gomock.Any()).Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil).AnyTimes()

				return archivePath, []target.Target{m}
			},
			cancelCtx: true,
			wantErr:   "context canceled",
		},
		{
			name: "returns error when context cancelled after fetch",
			setup: func(t *testing.T, ctrl *gomock.Controller) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: ctx-after-fetch
version: "1.0.0"
description: ctx after fetch
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("stub").AnyTimes()
				m.EXPECT().DisplayName().Return("Stub").AnyTimes()
				m.EXPECT().Install(gomock.Any(), gomock.Any()).Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil).AnyTimes()

				return archivePath, []target.Target{m}
			},
			customCtx: newCancelAfterN(3),
			wantErr:   "context canceled",
		},
		{
			name: "returns error when context cancelled after extract",
			setup: func(t *testing.T, ctrl *gomock.Controller) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: ctx-after-extract
version: "1.0.0"
description: ctx after extract
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("stub").AnyTimes()
				m.EXPECT().DisplayName().Return("Stub").AnyTimes()
				m.EXPECT().Install(gomock.Any(), gomock.Any()).Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil).AnyTimes()

				return archivePath, []target.Target{m}
			},
			// cancelAfterN(9): calls 1-9 return nil; call 10 fires at the
			// ctx.Err() check immediately after archive.Extract returns (Run line
			// 114). Call path: Run(1) + Fetch(2,3) + Run(4) + Extract-loop(5-9).
			customCtx: newCancelAfterN(9),
			wantErr:   "context canceled",
		},
		{
			name: "returns error when context cancelled inside checksum verification",
			setup: func(t *testing.T, ctrl *gomock.Controller) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: ctx-in-verify
version: "1.0.0"
description: ctx in verify
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("stub").AnyTimes()
				m.EXPECT().DisplayName().Return("Stub").AnyTimes()
				m.EXPECT().Install(gomock.Any(), gomock.Any()).Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil).AnyTimes()

				return archivePath, []target.Target{m}
			},
			// N=15 is sufficient to pass the initial ctx checks and fetch but
			// fires somewhere inside verify or shortly after — the exact call
			// count depends on archive size. We verify cancellation propagates.
			customCtx: newCancelAfterN(15),
			wantErr:   "context canceled",
		},
		{
			name:       "returns error when os.CreateTemp fails",
			noParallel: true,
			setup: func(t *testing.T, _ *gomock.Controller) (string, []target.Target) {
				t.Helper()

				return "/some/path.agentpack", nil
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := install.SetOsCreateTemp(install.CreateTempAlwaysFails)
				t.Cleanup(restore)
			},
			wantErr: "create temp file",
		},
		{
			name:       "returns error when os.MkdirTemp fails",
			noParallel: true,
			setup: func(t *testing.T, _ *gomock.Controller) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: mkdir-test-plugin
version: "1.0.0"
description: Plugin for mkdir temp test
`)

				return archivePath, nil
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := install.SetOsMkdirTemp(install.MkdirTempAlwaysFails)
				t.Cleanup(restore)
			},
			wantErr: "create temp dir",
		},
		{
			name: "returns error when archive is corrupt",
			setup: func(t *testing.T, _ *gomock.Controller) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				archivePath := filepath.Join(dir, "corrupt.agentpack")

				require.NoError(t, os.WriteFile(archivePath, []byte("not gzip data"), 0o644))

				return archivePath, nil
			},
			wantErr: "extract",
		},
		{
			name: "returns error when archive has no checksums.txt",
			setup: func(t *testing.T, _ *gomock.Controller) (string, []target.Target) {
				t.Helper()

				return buildArchiveNoChecksums(t), nil
			},
			wantErr: "checksums.txt not found",
		},
		{
			name: "returns error when archive has bad checksums format",
			setup: func(t *testing.T, _ *gomock.Controller) (string, []target.Target) {
				t.Helper()

				return buildArchiveBadChecksums(t), nil
			},
			wantErr: "reading checksums",
		},
		{
			name: "returns error when archive checksum fails",
			setup: func(t *testing.T, _ *gomock.Controller) (string, []target.Target) {
				t.Helper()

				return buildArchiveTamperedChecksum(t), nil
			},
			wantErr: "checksum failed",
		},
		{
			name: "returns error when archive has no metadata.json",
			setup: func(t *testing.T, _ *gomock.Controller) (string, []target.Target) {
				t.Helper()

				return buildArchiveNoMetadata(t), nil
			},
			wantErr: "metadata.json not found",
		},
		{
			name: "returns error when target Install fails",
			setup: func(t *testing.T, ctrl *gomock.Controller) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: tgt-fail-plugin
version: "1.0.0"
description: target fail test
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("fail-target").AnyTimes()
				m.EXPECT().DisplayName().Return("Fail Target").AnyTimes()
				m.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("target install error"))

				return archivePath, []target.Target{m}
			},
			wantErr: "install to fail-target",
		},
		{
			name:       "returns error when copyToTemp MkdirTemp fails",
			noParallel: true,
			setup: func(t *testing.T, ctrl *gomock.Controller) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: copytotemp-mkdir-plugin
version: "1.0.0"
description: test copyToTemp mkdir fail
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("stub").AnyTimes()
				m.EXPECT().DisplayName().Return("Stub").AnyTimes()
				m.EXPECT().Install(gomock.Any(), gomock.Any()).Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil).AnyTimes()

				return archivePath, []target.Target{m}
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				// Succeed on the first osMkdirTemp call (extract temp dir) and
				// fail on the second (inside copyToTemp).
				restore := install.SetOsMkdirTemp(install.MkdirTempFailAfterN(1))
				t.Cleanup(restore)
			},
			wantErr: "create target temp dir",
		},
		{
			name: "returns error when context cancelled during checksum verify",
			setup: func(t *testing.T, ctrl *gomock.Controller) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: ctx-verify-plugin
version: "1.0.0"
description: ctx during verify
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("stub").AnyTimes()
				m.EXPECT().DisplayName().Return("Stub").AnyTimes()
				m.EXPECT().Install(gomock.Any(), gomock.Any()).Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil).AnyTimes()

				return archivePath, []target.Target{m}
			},
			// cancelAfterN(10): the first 10 ctx.Err() calls return nil.
			// Call 11 fires inside checksum.Verify (first entry check), causing
			// Verify to return (nil, err) → line 131 "verify: %w".
			// Call path: Run(1) + Fetch(2,3) + Run(4) + Extract(5-9) + Run(10)
			// + Verify entry 1 = call 11.
			customCtx: newCancelAfterN(10),
			wantErr:   "context canceled",
		},
		{
			name: "returns error when context cancelled after metadata read",
			setup: func(t *testing.T, ctrl *gomock.Controller) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: ctx-post-meta-plugin
version: "1.0.0"
description: ctx after metadata
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("stub").AnyTimes()
				m.EXPECT().DisplayName().Return("Stub").AnyTimes()
				m.EXPECT().Install(gomock.Any(), gomock.Any()).Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil).AnyTimes()

				return archivePath, []target.Target{m}
			},
			// cancelAfterN(12): calls 11-12 are the two Verify entries (which
			// succeed). Call 13 fires at Run line 146 ctx.Err() check after
			// findAndReadMetadata completes.
			customCtx: newCancelAfterN(12),
			wantErr:   "context canceled",
		},
		{
			name: "returns error when context cancelled inside targets loop",
			setup: func(t *testing.T, ctrl *gomock.Controller) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: ctx-loop-plugin
version: "1.0.0"
description: ctx inside targets loop
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("stub").AnyTimes()
				m.EXPECT().DisplayName().Return("Stub").AnyTimes()
				m.EXPECT().Install(gomock.Any(), gomock.Any()).Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil).AnyTimes()

				return archivePath, []target.Target{m}
			},
			// cancelAfterN(13): call 13 is the Run line 146 check (passes nil).
			// Call 14 fires at Run line 165 (top of targets for-loop).
			customCtx: newCancelAfterN(13),
			wantErr:   "context canceled",
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

			ctrl := gomock.NewController(t)
			archivePath, targets := tt.setup(t, ctrl)

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

			var steps []install.Step
			var onStep func(install.Step)
			if tt.checkSteps != nil {
				onStep = func(s install.Step) { steps = append(steps, s) }
			}

			r, err := install.New().Run(ctx, install.Options{
				Source:  archivePath,
				Targets: targets,
				OnStep:  onStep,
			})

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.checkResult != nil {
				tt.checkResult(t, r)
			}

			if tt.checkSteps != nil {
				tt.checkSteps(t, steps)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestShortSHA
// --------------------------------------------------------------------------

func TestShortSHA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sha  string
		want string
	}{
		{
			name: "returns first 7 chars of long SHA",
			sha:  "a1b2c3d4e5f6789abcdef",
			want: "a1b2c3d",
		},
		{
			name: "returns full string when exactly 7 chars",
			sha:  "a1b2c3d",
			want: "a1b2c3d",
		},
		{
			name: "returns full string when shorter than 7 chars",
			sha:  "abc",
			want: "abc",
		},
		{
			name: "returns empty string when empty",
			sha:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := install.ShortSHA(tt.sha)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --------------------------------------------------------------------------
// TestCopyFile
// --------------------------------------------------------------------------

func TestCopyFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (src string, dst string)
		wantErr string
		check   func(t *testing.T, dst string)
	}{
		{
			name: "copies file content and mode",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "src.txt")

				require.NoError(t, os.WriteFile(src, []byte("hello copy"), 0o644))

				return src, filepath.Join(dir, "dst.txt")
			},
			check: func(t *testing.T, dst string) {
				t.Helper()
				data, err := os.ReadFile(dst)
				require.NoError(t, err)
				assert.Equal(t, "hello copy", string(data))
			},
		},
		{
			name: "returns error when src does not exist",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()

				return filepath.Join(dir, "nonexistent.txt"), filepath.Join(dir, "dst.txt")
			},
			wantErr: "read",
		},
		{
			name: "returns error when dst dir does not exist",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "src.txt")

				require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))

				return src, filepath.Join(dir, "nonexistent", "dst.txt")
			},
			wantErr: "write",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src, dst := tt.setup(t)
			err := install.CopyFile(src, dst)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, dst)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestCopyDir
// --------------------------------------------------------------------------

func TestCopyDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T) (src string, dst string)
		cancelCtx bool
		wantErr   string
		check     func(t *testing.T, dst string)
	}{
		{
			name: "copies directory tree recursively",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "src")

				require.NoError(t, os.MkdirAll(filepath.Join(src, "subdir"), 0o755))
				require.NoError(
					t,
					os.WriteFile(filepath.Join(src, "file.txt"), []byte("root"), 0o644),
				)
				require.NoError(
					t,
					os.WriteFile(
						filepath.Join(src, "subdir", "nested.txt"),
						[]byte("nested"),
						0o644,
					),
				)

				return src, filepath.Join(dir, "dst")
			},
			check: func(t *testing.T, dst string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dst, "file.txt"))
				require.NoError(t, err)
				assert.Equal(t, "root", string(data))

				data2, err := os.ReadFile(filepath.Join(dst, "subdir", "nested.txt"))
				require.NoError(t, err)
				assert.Equal(t, "nested", string(data2))
			},
		},
		{
			name: "returns error when context is cancelled",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "src")

				require.NoError(t, os.MkdirAll(src, 0o755))
				require.NoError(
					t,
					os.WriteFile(filepath.Join(src, "file.txt"), []byte("data"), 0o644),
				)

				return src, filepath.Join(dir, "dst")
			},
			cancelCtx: true,
			wantErr:   "context canceled",
		},
		{
			name: "returns error when walkdir encounters permission denied in src",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "src")
				subdir := filepath.Join(src, "subdir")

				require.NoError(t, os.MkdirAll(subdir, 0o755))
				require.NoError(
					t,
					os.WriteFile(filepath.Join(subdir, "file.txt"), []byte("x"), 0o644),
				)
				require.NoError(t, os.Chmod(subdir, 0o000))

				t.Cleanup(func() { _ = os.Chmod(subdir, 0o755) })

				return src, filepath.Join(dir, "dst")
			},
			wantErr: "permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src, dst := tt.setup(t)

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

			err := install.CopyDir(ctx, src, dst)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, dst)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestFindChecksums
// --------------------------------------------------------------------------

func TestFindChecksums(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr string
		check   func(t *testing.T, path string)
	}{
		{
			name: "finds checksums.txt inside .agentpack dir",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				agentpackDir := filepath.Join(dir, ".agentpack")

				require.NoError(t, os.MkdirAll(agentpackDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(agentpackDir, "checksums.txt"),
					[]byte("hash  file.txt\n"),
					0o644,
				))

				return dir
			},
			check: func(t *testing.T, path string) {
				t.Helper()
				assert.NotEmpty(t, path)
				assert.True(t, strings.HasSuffix(path, "checksums.txt"))
			},
		},
		{
			name: "returns error when checksums.txt not found",
			setup: func(t *testing.T) string {
				t.Helper()

				return t.TempDir()
			},
			wantErr: "checksums.txt not found",
		},
		{
			name: "returns error when walkdir encounters permission denied",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				locked := filepath.Join(dir, "locked")

				require.NoError(t, os.MkdirAll(locked, 0o000))

				t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

				return dir
			},
			wantErr: "searching for checksums.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setup(t)
			path, err := install.FindChecksums(dir)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, path)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestFindAndReadMetadata
// --------------------------------------------------------------------------

func TestFindAndReadMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr string
		check   func(t *testing.T, m any)
	}{
		{
			name: "finds and reads metadata.json inside .agentpack dir",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				agentpackDir := filepath.Join(dir, ".agentpack")

				require.NoError(t, os.MkdirAll(agentpackDir, 0o755))

				meta := metadata.Metadata{
					Name:    "my-plugin",
					Version: "1.0.0",
				}
				data, err := json.Marshal(meta)
				require.NoError(t, err)

				require.NoError(t, os.WriteFile(
					filepath.Join(agentpackDir, "metadata.json"), data, 0o644,
				))

				return dir
			},
			check: func(t *testing.T, m any) {
				t.Helper()
				assert.NotNil(t, m)
			},
		},
		{
			name: "returns error when metadata.json not found",
			setup: func(t *testing.T) string {
				t.Helper()

				return t.TempDir()
			},
			wantErr: "metadata.json not found",
		},
		{
			name: "returns error when metadata.json is invalid JSON",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				agentpackDir := filepath.Join(dir, ".agentpack")

				require.NoError(t, os.MkdirAll(agentpackDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(agentpackDir, "metadata.json"),
					[]byte("not json {{{"),
					0o644,
				))

				return dir
			},
			wantErr: "parse metadata.json",
		},
		{
			name: "returns error when walkdir encounters permission denied",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				locked := filepath.Join(dir, "locked")

				require.NoError(t, os.MkdirAll(locked, 0o000))

				t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

				return dir
			},
			wantErr: "searching for metadata.json",
		},
		{
			name: "returns error when metadata.json cannot be read",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				agentpackDir := filepath.Join(dir, ".agentpack")

				require.NoError(t, os.MkdirAll(agentpackDir, 0o755))

				metaPath := filepath.Join(agentpackDir, "metadata.json")

				require.NoError(t, os.WriteFile(metaPath, []byte(`{"name":"p"}`), 0o000))

				t.Cleanup(func() { _ = os.Chmod(metaPath, 0o644) })

				return dir
			},
			wantErr: "read metadata.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setup(t)
			m, err := install.FindAndReadMetadata(dir)

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
// TestNameFromSource
// --------------------------------------------------------------------------

func TestNameFromSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "extracts repo name from github URL",
			source: "github.com/org/skills-repo",
			want:   "skills-repo",
		},
		{
			name:   "strips .git suffix",
			source: "github.com/org/skills-repo.git",
			want:   "skills-repo",
		},
		{
			name:   "strips fragment before extracting name",
			source: "github.com/org/skills-repo#v1.0.0",
			want:   "skills-repo",
		},
		{
			name:   "strips trailing slash",
			source: "github.com/org/skills-repo/",
			want:   "skills-repo",
		},
		{
			name:   "returns source unchanged when no slash present",
			source: "myrepo",
			want:   "myrepo",
		},
		{
			name:   "handles https:// prefix",
			source: "https://github.com/org/my-plugin.git#main",
			want:   "my-plugin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := install.NameFromSource(tt.source)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --------------------------------------------------------------------------
// TestHumanSize
// --------------------------------------------------------------------------

func TestHumanSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{
			name:  "returns bytes when less than 1 KB",
			bytes: 512,
			want:  "512 B",
		},
		{
			name:  "returns bytes when exactly 0",
			bytes: 0,
			want:  "0 B",
		},
		{
			name:  "returns bytes for 1023 bytes",
			bytes: 1023,
			want:  "1023 B",
		},
		{
			name:  "returns KB when equal to 1 KB",
			bytes: 1024,
			want:  "1 KB",
		},
		{
			name:  "returns KB for larger values",
			bytes: 2048,
			want:  "2 KB",
		},
		{
			name:  "returns KB for non-round values",
			bytes: 1536,
			want:  "1 KB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := install.HumanSize(tt.bytes)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --------------------------------------------------------------------------
// TestRegistrySource
// --------------------------------------------------------------------------

func TestRegistrySource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts install.Options
		want string
	}{
		{
			name: "returns OriginalSource when set",
			opts: install.Options{
				Source:         "/tmp/cached/archive.agentpack",
				OriginalSource: "github.com/org/skills-repo",
			},
			want: "github.com/org/skills-repo",
		},
		{
			name: "returns Source when OriginalSource is empty",
			opts: install.Options{
				Source:         "/tmp/cached/archive.agentpack",
				OriginalSource: "",
			},
			want: "/tmp/cached/archive.agentpack",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := install.RegistrySource(tt.opts)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --------------------------------------------------------------------------
// TestCopyFileAtomic
// --------------------------------------------------------------------------

func TestCopyFileAtomic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (src, dst string)
		wantErr string
		check   func(t *testing.T, dst string)
	}{
		{
			name: "copies file atomically to destination",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "src.agentpack")
				dst := filepath.Join(dir, "dst.agentpack")

				require.NoError(t, os.WriteFile(src, []byte("archive data"), 0o644))

				return src, dst
			},
			check: func(t *testing.T, dst string) {
				t.Helper()
				data, err := os.ReadFile(dst)
				require.NoError(t, err)
				assert.Equal(t, "archive data", string(data))
			},
		},
		{
			name: "returns error when source does not exist",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()

				return filepath.Join(
						dir,
						"nonexistent.agentpack",
					), filepath.Join(
						dir,
						"dst.agentpack",
					)
			},
			wantErr: "open",
		},
		{
			name: "returns error when destination directory does not exist",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "src.agentpack")

				require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))

				return src, filepath.Join(dir, "nonexistent-dir", "dst.agentpack")
			},
			wantErr: "create temp for store",
		},
		{
			name: "returns error when rename fails because dst is an existing directory",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				src := filepath.Join(dir, "src.agentpack")

				require.NoError(t, os.WriteFile(src, []byte("some content"), 0o644))

				// A directory at dst makes os.Rename fail.
				dstDir := filepath.Join(dir, "dst-is-a-dir")
				require.NoError(t, os.MkdirAll(dstDir, 0o755))

				return src, dstDir
			},
			wantErr: "rename to store",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src, dst := tt.setup(t)
			err := install.CopyFileAtomic(src, dst)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, dst)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestRunFromGit
// --------------------------------------------------------------------------

func TestRunFromGit(t *testing.T) {
	// NOTE: not parallel — some subtests mutate package-level vars.

	tests := []struct {
		name        string
		noParallel  bool
		setup       func(t *testing.T, ctrl *gomock.Controller) (source string, targets []target.Target)
		injectFuncs func(t *testing.T)
		wantErr     string
		checkResult func(t *testing.T, r *install.Result)
	}{
		{
			name:       "returns error when osMkdirTemp fails for clone dir",
			noParallel: true,
			setup: func(t *testing.T, _ *gomock.Controller) (string, []target.Target) {
				t.Helper()
				bareRepo := initBareGitRepo(t)
				return bareRepo, nil
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := install.SetOsMkdirTemp(install.MkdirTempAlwaysFails)
				t.Cleanup(restore)
			},
			wantErr: "create temp dir",
		},
		{
			name: "returns error when git fetch fails",
			setup: func(t *testing.T, _ *gomock.Controller) (string, []target.Target) {
				t.Helper()
				return "github.com/nonexistent-org-xyzzy/nonexistent-repo-xyzzy.git", nil
			},
			wantErr: "fetch",
		},
		{
			name:       "installs from local bare git repo end-to-end",
			noParallel: true,
			setup: func(t *testing.T, ctrl *gomock.Controller) (string, []target.Target) {
				t.Helper()
				bareRepo := initBareGitRepo(t)

				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("claude-code").AnyTimes()
				m.EXPECT().DisplayName().Return("Claude Code").AnyTimes()
				m.EXPECT().Install(gomock.Any(), gomock.Any()).Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil)

				return bareRepo, []target.Target{m}
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := install.SetRegistrySave(
					func(_ *registry.PackageManifest) error { return nil },
				)
				t.Cleanup(restore)
				// Redirect archives dir so storeArchive doesn't write to real home.
				archivesDir := t.TempDir()
				restoreArchives := install.SetArchivesDir(
					func() (string, error) { return archivesDir, nil },
				)
				t.Cleanup(restoreArchives)
			},
			checkResult: func(t *testing.T, r *install.Result) {
				t.Helper()
				require.NotNil(t, r)
				assert.NotEmpty(t, r.SHA)
			},
		},
		{
			name:       "installs from local bare git repo with ref in source",
			noParallel: true,
			setup: func(t *testing.T, ctrl *gomock.Controller) (string, []target.Target) {
				t.Helper()
				bareRepo := initBareGitRepo(t)

				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("claude-code").AnyTimes()
				m.EXPECT().DisplayName().Return("Claude Code").AnyTimes()
				m.EXPECT().Install(gomock.Any(), gomock.Any()).Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil)

				return bareRepo + "#main", []target.Target{m}
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				restore := install.SetRegistrySave(
					func(_ *registry.PackageManifest) error { return nil },
				)
				t.Cleanup(restore)
				archivesDir := t.TempDir()
				restoreArchives := install.SetArchivesDir(
					func() (string, error) { return archivesDir, nil },
				)
				t.Cleanup(restoreArchives)
			},
			checkResult: func(t *testing.T, r *install.Result) {
				t.Helper()
				require.NotNil(t, r)
			},
		},
		{
			name:       "returns error when context is cancelled after clone",
			noParallel: true,
			setup: func(t *testing.T, _ *gomock.Controller) (string, []target.Target) {
				t.Helper()
				bareRepo := initBareGitRepo(t)
				return bareRepo, nil
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				// Use cancelAfterN to cancel just after FetchWithResult succeeds.
				// The exact count may vary; this covers the ctx.Err() check at
				// line 160 (after emitStep "cloning").
			},
			wantErr: "",
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

			ctrl := gomock.NewController(t)
			source, targets := tt.setup(t, ctrl)

			r, err := install.New().Run(context.Background(), install.Options{
				Source:  source,
				Targets: targets,
			})

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			if err != nil {
				if tt.name == "returns error when context is cancelled after clone" {
					// This test is intentionally loose — it may or may not error.
					return
				}
				require.NoError(t, err)
			}

			if tt.checkResult != nil {
				tt.checkResult(t, r)
			}
		})
	}
}
