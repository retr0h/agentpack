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

	"github.com/avfs/avfs/vfs/osfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	"github.com/retr0h/agentpack/internal/archive"
	"github.com/retr0h/agentpack/internal/lock"
	"github.com/retr0h/agentpack/internal/metadata"
	"github.com/retr0h/agentpack/internal/packages"
	"github.com/retr0h/agentpack/internal/target"
	"github.com/retr0h/agentpack/internal/target/mocks"
	"github.com/retr0h/agentpack/internal/testutil"
	"github.com/retr0h/agentpack/pkg/build"
	"github.com/retr0h/agentpack/pkg/install"
	"github.com/retr0h/agentpack/pkg/registry"
	"github.com/retr0h/agentpack/pkg/remove"
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
		_, err := cmd.CombinedOutput()
		require.NoError(t, err)
	}

	run("clone", "--bare", src, bare)

	return bare
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
		_, err := cmd.CombinedOutput()
		require.NoError(t, err)
	}

	run("init")
	run("checkout", "-b", "main")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644))

	skillDir := filepath.Join(dir, "skills", "test-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Test Skill\n"), 0o644),
	)

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

// buildArchiveWithEntries builds an archive containing skills and commands with
// a metadata.yaml that includes explicit entries (ADR-009 format).
func buildArchiveWithEntries(t *testing.T, name, version string) string {
	t.Helper()

	skillContent := []byte("# Review Skill\n")
	commandContent := []byte("# scan command\n")

	meta := metadata.Metadata{
		Name:           name,
		Version:        version,
		GitCommitSHA:   "abc1234567890",
		BuildTimestamp: "2026-01-01T00:00:00Z",
		BuilderVersion: "dev",
		Platform:       "darwin-arm64",
		Entries: []metadata.ContentEntry{
			{Name: "review", Type: "skill"},
			{Name: "commands", Type: "command"},
		},
	}

	metaYAML, err := yaml.Marshal(meta)
	require.NoError(t, err)

	dir := t.TempDir()
	vfs := osfs.NewWithNoIdm()
	outPath := filepath.Join(dir, name+".agentpack")

	require.NoError(t, archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
		{ArchivePath: "skills/review/SKILL.md", Content: skillContent},
		{ArchivePath: "commands/scan.md", Content: commandContent},
		{ArchivePath: ".agentpack/metadata.yaml", Content: metaYAML},
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
		global      bool
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
skills:
  - skills/**/*
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("test").AnyTimes()
				m.EXPECT().DisplayName().Return("Test").AnyTimes()
				m.EXPECT().
					SupportedTypes().
					Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
					AnyTimes()
				m.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil)

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
			name:       "filters entries by target SupportedTypes",
			noParallel: true,
			setup: func(t *testing.T, ctrl *gomock.Controller) (string, []target.Target) {
				t.Helper()
				archivePath := buildArchiveWithEntries(t, "filter-entries-plugin", "1.0.0")

				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("skill-only").AnyTimes()
				m.EXPECT().DisplayName().Return("Skill Only").AnyTimes()
				m.EXPECT().
					SupportedTypes().
					Return([]string{"skill"}).
					AnyTimes()
				m.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, opts target.InstallOpts) ([]target.InstalledFile, error) {
						// Verify only skill entries are passed.
						require.Len(t, opts.Entries, 1)
						assert.Equal(t, "review", opts.Entries[0].Name)
						assert.Equal(t, "skill", opts.Entries[0].Type)
						assert.Contains(t, opts.Entries[0].Root, filepath.Join("skills", "review"))

						return []target.InstalledFile{
							{Path: "skills/review/SKILL.md", SHA256: "dummy"},
						}, nil
					})

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
				assert.Equal(t, "filter-entries-plugin", r.Name)
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
skills:
  - skills/**/*
`)
				m1 := mocks.NewMockTarget(ctrl)
				m1.EXPECT().Name().Return("alpha").AnyTimes()
				m1.EXPECT().DisplayName().Return("Alpha").AnyTimes()
				m1.EXPECT().
					SupportedTypes().
					Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
					AnyTimes()
				m1.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil)

				m2 := mocks.NewMockTarget(ctrl)
				m2.EXPECT().Name().Return("beta").AnyTimes()
				m2.EXPECT().DisplayName().Return("Beta").AnyTimes()
				m2.EXPECT().
					SupportedTypes().
					Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
					AnyTimes()
				m2.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil)

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
skills:
  - skills/**/*
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("step-target").AnyTimes()
				m.EXPECT().DisplayName().Return("Step Target").AnyTimes()
				m.EXPECT().
					SupportedTypes().
					Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
					AnyTimes()
				m.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil)

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
skills:
  - skills/**/*
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("stub").AnyTimes()
				m.EXPECT().DisplayName().Return("Stub").AnyTimes()
				m.EXPECT().
					SupportedTypes().
					Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
					AnyTimes()
				m.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil).
					AnyTimes()

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
skills:
  - skills/**/*
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("stub").AnyTimes()
				m.EXPECT().DisplayName().Return("Stub").AnyTimes()
				m.EXPECT().
					SupportedTypes().
					Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
					AnyTimes()
				m.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil).
					AnyTimes()

				return archivePath, []target.Target{m}
			},
			customCtx: testutil.NewCancelAfterN(3),
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
skills:
  - skills/**/*
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("stub").AnyTimes()
				m.EXPECT().DisplayName().Return("Stub").AnyTimes()
				m.EXPECT().
					SupportedTypes().
					Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
					AnyTimes()
				m.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil).
					AnyTimes()

				return archivePath, []target.Target{m}
			},
			// cancelAfterN(9): calls 1-9 return nil; call 10 fires at the
			// ctx.Err() check immediately after archive.Extract returns (Run line
			// 114). Call path: Run(1) + Fetch(2,3) + Run(4) + Extract-loop(5-9).
			customCtx: testutil.NewCancelAfterN(9),
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
skills:
  - skills/**/*
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("stub").AnyTimes()
				m.EXPECT().DisplayName().Return("Stub").AnyTimes()
				m.EXPECT().
					SupportedTypes().
					Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
					AnyTimes()
				m.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil).
					AnyTimes()

				return archivePath, []target.Target{m}
			},
			// N=15 is sufficient to pass the initial ctx checks and fetch but
			// fires somewhere inside verify or shortly after — the exact call
			// count depends on archive size. We verify cancellation propagates.
			customCtx: testutil.NewCancelAfterN(15),
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
skills:
  - skills/**/*
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
skills:
  - skills/**/*
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("fail-target").AnyTimes()
				m.EXPECT().DisplayName().Return("Fail Target").AnyTimes()
				m.EXPECT().
					SupportedTypes().
					Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
					AnyTimes()
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
skills:
  - skills/**/*
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("stub").AnyTimes()
				m.EXPECT().DisplayName().Return("Stub").AnyTimes()
				m.EXPECT().
					SupportedTypes().
					Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
					AnyTimes()
				m.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil).
					AnyTimes()

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
skills:
  - skills/**/*
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("stub").AnyTimes()
				m.EXPECT().DisplayName().Return("Stub").AnyTimes()
				m.EXPECT().
					SupportedTypes().
					Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
					AnyTimes()
				m.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil).
					AnyTimes()

				return archivePath, []target.Target{m}
			},
			// cancelAfterN(10): the first 10 ctx.Err() calls return nil.
			// Call 11 fires inside checksum.Verify (first entry check), causing
			// Verify to return (nil, err) → line 131 "verify: %w".
			// Call path: Run(1) + Fetch(2,3) + Run(4) + Extract(5-9) + Run(10)
			// + Verify entry 1 = call 11.
			customCtx: testutil.NewCancelAfterN(10),
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
skills:
  - skills/**/*
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("stub").AnyTimes()
				m.EXPECT().DisplayName().Return("Stub").AnyTimes()
				m.EXPECT().
					SupportedTypes().
					Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
					AnyTimes()
				m.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil).
					AnyTimes()

				return archivePath, []target.Target{m}
			},
			// cancelAfterN(12): calls 11-12 are the two Verify entries (which
			// succeed). Call 13 fires at Run line 146 ctx.Err() check after
			// findAndReadMetadata completes.
			customCtx: testutil.NewCancelAfterN(12),
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
skills:
  - skills/**/*
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("stub").AnyTimes()
				m.EXPECT().DisplayName().Return("Stub").AnyTimes()
				m.EXPECT().
					SupportedTypes().
					Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
					AnyTimes()
				m.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil).
					AnyTimes()

				return archivePath, []target.Target{m}
			},
			// cancelAfterN(13): call 13 is the Run line 146 check (passes nil).
			// Call 14 fires at Run line 165 (top of targets for-loop).
			customCtx: testutil.NewCancelAfterN(13),
			wantErr:   "context canceled",
		},
		{
			name:       "installs with global scope sets ScopeGlobal in manifest",
			noParallel: true,
			global:     true,
			setup: func(t *testing.T, ctrl *gomock.Controller) (string, []target.Target) {
				t.Helper()
				dir := t.TempDir()
				initGitRepo(t, dir)
				archivePath := buildTestArchive(t, dir, `
name: global-plugin
version: "1.0.0"
description: Global scope test
skills:
  - skills/**/*
`)
				m := mocks.NewMockTarget(ctrl)
				m.EXPECT().Name().Return("global-test").AnyTimes()
				m.EXPECT().DisplayName().Return("Global Test").AnyTimes()
				m.EXPECT().
					SupportedTypes().
					Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
					AnyTimes()
				m.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil)

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
				assert.Equal(t, "global-plugin", r.Name)
			},
		},
		{
			name: "returns error when archive has metadata but no content dirs",
			setup: func(t *testing.T, _ *gomock.Controller) (string, []target.Target) {
				t.Helper()
				// Build an archive with metadata.yaml but no skills/commands/etc. dirs.
				// Using the new YAML format so no checksums are required.
				dir := t.TempDir()
				vfs := osfs.NewWithNoIdm()
				outPath := filepath.Join(dir, "nocontent.agentpack")

				meta := metadata.Metadata{
					Name:    "nocontent-plugin",
					Version: "1.0.0",
				}
				metaYAML, err := yaml.Marshal(meta)
				require.NoError(t, err)

				require.NoError(
					t,
					archive.Create(context.Background(), vfs, outPath, []archive.FileEntry{
						{ArchivePath: ".agentpack/metadata.yaml", Content: metaYAML},
					}),
				)

				return outPath, nil
			},
			wantErr: "has no installable content",
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

			r, err := install.NewWithTargets(targets).Run(ctx, install.Options{
				Source: archivePath,
				OnStep: onStep,
				Global: tt.global,
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
			path, err := install.FindChecksums(context.Background(), dir)

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
		{
			name: "finds and reads metadata.yaml inside .agentpack dir",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				agentpackDir := filepath.Join(dir, ".agentpack")

				require.NoError(t, os.MkdirAll(agentpackDir, 0o755))

				meta := metadata.Metadata{
					Name:    "yaml-plugin",
					Version: "2.0.0",
				}
				data, err := yaml.Marshal(meta)
				require.NoError(t, err)

				require.NoError(t, os.WriteFile(
					filepath.Join(agentpackDir, "metadata.yaml"), data, 0o644,
				))

				return dir
			},
			check: func(t *testing.T, m any) {
				t.Helper()
				assert.NotNil(t, m)
			},
		},
		{
			name: "returns error when metadata.yaml cannot be read",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				agentpackDir := filepath.Join(dir, ".agentpack")

				require.NoError(t, os.MkdirAll(agentpackDir, 0o755))

				metaPath := filepath.Join(agentpackDir, "metadata.yaml")

				require.NoError(t, os.WriteFile(metaPath, []byte("name: p\n"), 0o000))

				t.Cleanup(func() { _ = os.Chmod(metaPath, 0o644) })

				return dir
			},
			wantErr: "read metadata.yaml",
		},
		{
			name: "returns error when metadata.yaml has invalid YAML",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				agentpackDir := filepath.Join(dir, ".agentpack")

				require.NoError(t, os.MkdirAll(agentpackDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(agentpackDir, "metadata.yaml"),
					[]byte("name: test\n\tinvalid: yaml\n"),
					0o644,
				))

				return dir
			},
			wantErr: "parse metadata.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setup(t)
			m, err := install.FindAndReadMetadata(context.Background(), dir)

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
			name:   "returns owner/repo from github URL",
			source: "github.com/jeffallan/claude-skills",
			want:   "jeffallan/claude-skills",
		},
		{
			name:   "returns owner/repo from github URL with different org",
			source: "github.com/org/skills-repo",
			want:   "org/skills-repo",
		},
		{
			name:   "strips .git suffix before extracting owner/repo",
			source: "github.com/org/skills-repo.git",
			want:   "org/skills-repo",
		},
		{
			name:   "strips fragment before extracting owner/repo",
			source: "github.com/org/skills-repo#v1.0.0",
			want:   "org/skills-repo",
		},
		{
			name:   "strips trailing slash before extracting owner/repo",
			source: "github.com/org/skills-repo/",
			want:   "org/skills-repo",
		},
		{
			name:   "returns base filename for local path without extension",
			source: "myrepo",
			want:   "myrepo",
		},
		{
			name:   "strips https scheme and returns owner/repo",
			source: "https://github.com/org/my-plugin.git#main",
			want:   "org/my-plugin",
		},
		{
			name:   "returns owner/repo from gitlab URL",
			source: "gitlab.com/org/repo#v1.0",
			want:   "org/repo",
		},
		{
			name:   "returns filename without extension for local agentpack archive",
			source: "/local/path/file.agentpack",
			want:   "file",
		},
		{
			name:   "returns owner/repo from https github URL without ref",
			source: "https://github.com/org/repo.git",
			want:   "org/repo",
		},
		{
			name:   "returns bare name without extension for name with dot extension",
			source: "myrepo.agentpack",
			want:   "myrepo",
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
// TestParseSource
// --------------------------------------------------------------------------

func TestParseSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		raw           string
		wantSource    string
		wantRef       string
		wantSelectors []install.ContentSelector
	}{
		{
			name:       "bare owner/repo returns source only",
			raw:        "owner/repo",
			wantSource: "owner/repo",
		},
		{
			name:       "owner/repo with version pin",
			raw:        "owner/repo@v2.0.0",
			wantSource: "owner/repo",
			wantRef:    "v2.0.0",
		},
		{
			name:       "owner/repo with skill selector",
			raw:        "owner/repo:skill/k8s",
			wantSource: "owner/repo",
			wantSelectors: []install.ContentSelector{
				{Type: "skill", Name: "k8s"},
			},
		},
		{
			name:       "owner/repo with version and selector",
			raw:        "owner/repo@v2.0.0:skill/k8s",
			wantSource: "owner/repo",
			wantRef:    "v2.0.0",
			wantSelectors: []install.ContentSelector{
				{Type: "skill", Name: "k8s"},
			},
		},
		{
			name:       "owner/repo with multiple selectors",
			raw:        "owner/repo:skill/k8s:command/scan",
			wantSource: "owner/repo",
			wantSelectors: []install.ContentSelector{
				{Type: "skill", Name: "k8s"},
				{Type: "command", Name: "scan"},
			},
		},
		{
			name:       "full format with version and multiple selectors",
			raw:        "owner/repo@v2.0.0:skill/k8s:command/scan",
			wantSource: "owner/repo",
			wantRef:    "v2.0.0",
			wantSelectors: []install.ContentSelector{
				{Type: "skill", Name: "k8s"},
				{Type: "command", Name: "scan"},
			},
		},
		{
			name:       "github.com full URL with ref",
			raw:        "github.com/org/repo@main",
			wantSource: "github.com/org/repo",
			wantRef:    "main",
		},
		{
			name:       "SHA ref",
			raw:        "owner/repo@abc1234",
			wantSource: "owner/repo",
			wantRef:    "abc1234",
		},
		{
			name:       "all six content types",
			raw:        "owner/repo:skill/a:command/b:hook/c:agent/d:mcp/e:config/f",
			wantSource: "owner/repo",
			wantSelectors: []install.ContentSelector{
				{Type: "skill", Name: "a"},
				{Type: "command", Name: "b"},
				{Type: "hook", Name: "c"},
				{Type: "agent", Name: "d"},
				{Type: "mcp", Name: "e"},
				{Type: "config", Name: "f"},
			},
		},
		{
			name:       "empty selector segments are skipped",
			raw:        "owner/repo:skill/k8s:",
			wantSource: "owner/repo",
			wantSelectors: []install.ContentSelector{
				{Type: "skill", Name: "k8s"},
			},
		},
		{
			name:       "selector without slash is skipped",
			raw:        "owner/repo:invalid",
			wantSource: "owner/repo",
		},
		{
			name:       "local path without selectors",
			raw:        "/path/to/file.agentpack",
			wantSource: "/path/to/file.agentpack",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec := install.ParseSource(tt.raw)
			assert.Equal(t, tt.wantSource, spec.Source)
			assert.Equal(t, tt.wantRef, spec.Ref)

			if tt.wantSelectors == nil {
				assert.Empty(t, spec.Selectors)
			} else {
				assert.Equal(t, tt.wantSelectors, spec.Selectors)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestSelectorsToContent
// --------------------------------------------------------------------------

func TestSelectorsToContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		selectors []install.ContentSelector
		want      []string
	}{
		{
			name: "converts selectors to type/name strings",
			selectors: []install.ContentSelector{
				{Type: "skill", Name: "k8s"},
				{Type: "command", Name: "scan"},
			},
			want: []string{"skill/k8s", "command/scan"},
		},
		{
			name:      "nil selectors returns nil",
			selectors: nil,
			want:      nil,
		},
		{
			name:      "empty selectors returns nil",
			selectors: []install.ContentSelector{},
			want:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := install.SelectorsToContent(tt.selectors)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --------------------------------------------------------------------------
// TestSelectorsToSkillFilter
// --------------------------------------------------------------------------

func TestSelectorsToSkillFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		selectors []install.ContentSelector
		want      []string
	}{
		{
			name: "extracts skill names from mixed selectors",
			selectors: []install.ContentSelector{
				{Type: "skill", Name: "k8s"},
				{Type: "command", Name: "scan"},
				{Type: "skill", Name: "react"},
			},
			want: []string{"k8s", "react"},
		},
		{
			name: "returns nil when no skill selectors",
			selectors: []install.ContentSelector{
				{Type: "command", Name: "scan"},
			},
			want: nil,
		},
		{
			name:      "nil selectors returns nil",
			selectors: nil,
			want:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := install.SelectorsToSkillFilter(tt.selectors)
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
				m.EXPECT().
					SupportedTypes().
					Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
					AnyTimes()
				m.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil)

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
				m.EXPECT().
					SupportedTypes().
					Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
					AnyTimes()
				m.EXPECT().
					Install(gomock.Any(), gomock.Any()).
					Return([]target.InstalledFile{{Path: "skills/test/SKILL.md", SHA256: "dummy"}}, nil)

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
		{
			name:       "returns error when cloned repo has no content dirs",
			noParallel: true,
			setup: func(t *testing.T, _ *gomock.Controller) (string, []target.Target) {
				t.Helper()
				// Create a bare repo with only a README (no skills/commands/etc.).
				dir := t.TempDir()
				src := filepath.Join(dir, "src")
				bare := filepath.Join(dir, "repo.git")

				require.NoError(t, os.MkdirAll(src, 0o755))

				run := func(args ...string) {
					t.Helper()
					cmd := exec.Command("git", args...)
					cmd.Dir = src
					cmd.Env = append(os.Environ(), gitEnv...)
					_, runErr := cmd.CombinedOutput()
					require.NoError(t, runErr)
				}

				run("init")
				run("checkout", "-b", "main")
				require.NoError(
					t,
					os.WriteFile(filepath.Join(src, "README.md"), []byte("hello\n"), 0o644),
				)
				run("add", ".")
				run("commit", "-m", "init")

				cloneCmd := exec.Command("git", "clone", "--bare", src, bare)
				cloneCmd.Env = append(os.Environ(), gitEnv...)
				_, cloneErr := cloneCmd.CombinedOutput()
				require.NoError(t, cloneErr)

				return bare, nil
			},
			injectFuncs: func(t *testing.T) {
				t.Helper()
				archivesDir := t.TempDir()
				restore := install.SetArchivesDir(
					func() (string, error) { return archivesDir, nil },
				)
				t.Cleanup(restore)
			},
			wantErr: "no installable content",
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

			r, err := install.NewWithTargets(targets).Run(context.Background(), install.Options{
				Source: source,
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

// --------------------------------------------------------------------------
// TestHasMetadataYAML
// --------------------------------------------------------------------------

func TestHasMetadataYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T) string
		want  bool
	}{
		{
			name: "returns true when .agentpack/metadata.yaml exists at top level",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				agentpackDir := filepath.Join(dir, ".agentpack")
				require.NoError(t, os.MkdirAll(agentpackDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(agentpackDir, "metadata.yaml"),
					[]byte("name: p\n"),
					0o644,
				))
				return dir
			},
			want: true,
		},
		{
			name: "returns false when no metadata.yaml present",
			setup: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setup(t)
			got := install.HasMetadataYAML(dir)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --------------------------------------------------------------------------
// TestHasContentDirs
// --------------------------------------------------------------------------

func TestHasContentDirs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T) string
		want  bool
	}{
		{
			name: "returns true when skills dir is present",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "skills"), 0o755))
				return dir
			},
			want: true,
		},
		{
			name: "returns true when commands dir is present",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "commands"), 0o755))
				return dir
			},
			want: true,
		},
		{
			name: "returns false when no known content dirs are present",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "README.md"),
					[]byte("hello\n"),
					0o644,
				))
				return dir
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setup(t)
			got := install.HasContentDirs(dir)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --------------------------------------------------------------------------
// TestMergeFiles
// --------------------------------------------------------------------------

func TestMergeFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		existing []registry.InstalledFile
		incoming []registry.InstalledFile
		wantLen  int
		check    func(t *testing.T, got []registry.InstalledFile)
	}{
		{
			name:     "returns incoming when existing is empty",
			existing: nil,
			incoming: []registry.InstalledFile{
				{Path: "skills/foo.md", Target: "claude-code", SHA256: "abc"},
			},
			wantLen: 1,
		},
		{
			name: "appends new entries to existing",
			existing: []registry.InstalledFile{
				{Path: "skills/foo.md", Target: "claude-code", SHA256: "abc"},
			},
			incoming: []registry.InstalledFile{
				{Path: "skills/bar.md", Target: "claude-code", SHA256: "def"},
			},
			wantLen: 2,
		},
		{
			name: "updates existing entry with same path and target",
			existing: []registry.InstalledFile{
				{Path: "skills/foo.md", Target: "claude-code", SHA256: "old"},
			},
			incoming: []registry.InstalledFile{
				{Path: "skills/foo.md", Target: "claude-code", SHA256: "new"},
			},
			wantLen: 1,
			check: func(t *testing.T, got []registry.InstalledFile) {
				t.Helper()
				assert.Equal(t, "new", got[0].SHA256)
			},
		},
		{
			name: "treats same path different target as distinct entries",
			existing: []registry.InstalledFile{
				{Path: "skills/foo.md", Target: "claude-code", SHA256: "abc"},
			},
			incoming: []registry.InstalledFile{
				{Path: "skills/foo.md", Target: "cursor", SHA256: "abc"},
			},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := install.MergeFiles(tt.existing, tt.incoming)
			assert.Len(t, got, tt.wantLen)

			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Lifecycle archive builder
// --------------------------------------------------------------------------

// lifecycleArchive builds a .agentpack archive containing a realistic layout:
//
//	skills/kubernetes-specialist/SKILL.md
//	skills/react-expert/SKILL.md
//	commands/scan.md
//	hooks/hooks.json
//	.agentpack/metadata.json
//	.agentpack/checksums.txt
//
// The archive is written to a temp file and its path is returned.
func lifecycleArchive(t *testing.T, name, version string) string {
	t.Helper()

	k8sSkill := []byte("# Kubernetes Specialist skill\n")
	reactSkill := []byte("# React Expert skill\n")
	scanCmd := []byte("# scan command\n")
	hooksJSON := []byte(`{"hooks":[]}`)

	metaJSON, err := json.Marshal(metadata.Metadata{
		Name:           name,
		Version:        version,
		GitCommitSHA:   "deadbeef1234567890abcdef",
		BuildTimestamp: "2026-01-01T00:00:00Z",
		BuilderVersion: "dev",
		Platform:       "darwin-arm64",
	})
	require.NoError(t, err)

	type fileSpec struct {
		path    string
		content []byte
	}

	contentFiles := []fileSpec{
		{"skills/kubernetes-specialist/SKILL.md", k8sSkill},
		{"skills/react-expert/SKILL.md", reactSkill},
		{"commands/scan.md", scanCmd},
		{"hooks/hooks.json", hooksJSON},
	}

	var sb strings.Builder
	for _, e := range contentFiles {
		fmt.Fprintf(&sb, "%s  %s\n", sha256HexBytes(e.content), e.path)
	}
	fmt.Fprintf(&sb, "%s  %s\n", sha256HexBytes(metaJSON), ".agentpack/metadata.json")
	checksumLines := sb.String()

	dir := t.TempDir()
	outPath := filepath.Join(dir, name+".agentpack")
	vfs := osfs.NewWithNoIdm()

	fileEntries := make([]archive.FileEntry, 0, len(contentFiles)+2)
	for _, e := range contentFiles {
		fileEntries = append(fileEntries, archive.FileEntry{
			ArchivePath: e.path,
			Content:     e.content,
		})
	}

	fileEntries = append(
		fileEntries,
		archive.FileEntry{ArchivePath: ".agentpack/metadata.json", Content: metaJSON},
		archive.FileEntry{ArchivePath: ".agentpack/checksums.txt", Content: []byte(checksumLines)},
	)

	require.NoError(t, archive.Create(context.Background(), vfs, outPath, fileEntries))

	return outPath
}

// sha256HexBytes returns the hex-encoded SHA256 of data.
func sha256HexBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// --------------------------------------------------------------------------
// Mock target that copies all non-agentpack files from SourceDir to installDir.
// --------------------------------------------------------------------------

// mockTargetThatInstalls returns a MockTarget whose Install method writes all
// non-.agentpack files from opts.SourceDir into installDir and returns
// InstalledFile records with correct SHA256 values. It may be called multiple
// times (AnyTimes).
func mockTargetThatInstalls(
	t *testing.T,
	ctrl *gomock.Controller,
	name, displayName string,
	installDir string,
) *mocks.MockTarget {
	t.Helper()

	m := mocks.NewMockTarget(ctrl)
	m.EXPECT().Name().Return(name).AnyTimes()
	m.EXPECT().DisplayName().Return(displayName).AnyTimes()
	m.EXPECT().
		SupportedTypes().
		Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
		AnyTimes()

	m.EXPECT().
		Install(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, opts target.InstallOpts) ([]target.InstalledFile, error) {
			var installed []target.InstalledFile

			err := filepath.WalkDir(
				opts.SourceDir,
				func(path string, d os.DirEntry, walkErr error) error {
					if walkErr != nil {
						return walkErr
					}

					if d.IsDir() {
						return nil
					}

					rel, relErr := filepath.Rel(opts.SourceDir, path)
					if relErr != nil {
						return relErr
					}

					// Skip .agentpack internal files -- targets never install those.
					if len(rel) >= len(".agentpack") && rel[:len(".agentpack")] == ".agentpack" {
						return nil
					}

					dst := filepath.Join(installDir, rel)
					if mkErr := os.MkdirAll(filepath.Dir(dst), 0o755); mkErr != nil {
						return mkErr
					}

					data, readErr := os.ReadFile(path)
					if readErr != nil {
						return readErr
					}

					if writeErr := os.WriteFile(dst, data, 0o644); writeErr != nil {
						return writeErr
					}

					h := sha256.Sum256(data)

					installed = append(installed, target.InstalledFile{
						Path:   rel,
						SHA256: hex.EncodeToString(h[:]),
					})

					return nil
				},
			)

			return installed, err
		}).
		AnyTimes()

	return m
}

// --------------------------------------------------------------------------
// Registry helpers
// --------------------------------------------------------------------------

// withTempRegistry redirects the package-level registry home to a temp dir and
// returns the *registry.Registry plus a cleanup function. The cleanup must be
// called (typically via defer) to restore the original home-dir function.
func withTempRegistry(t *testing.T) (*registry.Registry, func()) {
	t.Helper()

	home := t.TempDir()
	restore := registry.SetOsUserHomeDir(func() (string, error) { return home, nil })

	return registry.New(), restore
}

// --------------------------------------------------------------------------
// Assertion helpers
// --------------------------------------------------------------------------

// assertFilePath fails when no file in the slice matches wantPath (forward-slash).
func assertFilePath(t *testing.T, files []registry.InstalledFile, wantPath string) {
	t.Helper()
	found := false
	for _, f := range files {
		if filepath.ToSlash(f.Path) == wantPath {
			found = true
			break
		}
	}
	assert.True(t, found)
}

// assertNoFilePath fails when any file in the slice matches wantPath.
func assertNoFilePath(t *testing.T, files []registry.InstalledFile, wantPath string) {
	t.Helper()
	for _, f := range files {
		assert.NotEqual(t, wantPath, filepath.ToSlash(f.Path))
	}
}

// collectTargetNames returns the unique Target fields from a file list.
func collectTargetNames(files []registry.InstalledFile) []string {
	seen := make(map[string]bool)
	for _, f := range files {
		seen[f.Target] = true
	}

	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}

	return names
}

// --------------------------------------------------------------------------
// TestLifecycleFullAddListDelete
// --------------------------------------------------------------------------

// TestLifecycleFullAddListDelete exercises the complete install -> list -> delete
// lifecycle across two targets, a partial skill removal, and a full removal.
//
// This test is sequential (no t.Parallel) because it mutates the package-level
// swappable vars registrySave, registryLoad, and osUserHomeDir.
func TestLifecycleFullAddListDelete(t *testing.T) {
	tests := []struct {
		name       string
		noParallel bool
		run        func(t *testing.T)
	}{
		{
			name:       "full lifecycle: install two targets, list, partial delete, full delete",
			noParallel: true,
			run:        testLifecycleFull,
		},
		{
			name:       "whole repo install: no skill filter installs all content",
			noParallel: true,
			run:        testLifecycleWholeRepo,
		},
		{
			name:       "no targets detected: returns error",
			noParallel: true,
			run:        testLifecycleNoTargets,
		},
		{
			name:       "@skill filter: only skills content installed, commands and hooks absent",
			noParallel: true,
			run:        testLifecycleSkillFilter,
		},
		{
			name:       "whole repo: all content types present in registry",
			noParallel: true,
			run:        testLifecycleWholeRepoAllContent,
		},
		{
			name:       "@skill then whole repo: registry merges selected skills and all content",
			noParallel: true,
			run:        testLifecycleSkillThenWholeRepo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.noParallel {
				t.Parallel()
			}

			tt.run(t)
		})
	}
}

// testLifecycleFull exercises:
//  1. Install @kubernetes-specialist to claude-code -> assert registry entry
//  2. Install @react-expert to cursor (same pkg) -> assert merged registry
//  3. list.RunWithRegistry -> 1 entry, both targets, both skills
//  4. Partial delete @react-expert -> registry pruned, files removed from disk
//  5. Full delete -> registry entry gone
func testLifecycleFull(t *testing.T) {
	t.Helper()

	archivePath := lifecycleArchive(t, "lifecycle-pkg", "1.0.0")
	installDirCC := t.TempDir()
	installDirCursor := t.TempDir()

	reg, restoreHome := withTempRegistry(t)
	defer restoreHome()

	restoreSave := install.SetRegistrySave(reg.Save)
	defer restoreSave()

	restoreLoad := install.SetRegistryLoad(reg.Load)
	defer restoreLoad()

	ctx := context.Background()
	ctrl := gomock.NewController(t)

	// -------------------------------------------------------------------------
	// Step 1 & 2: Install @kubernetes-specialist to claude-code.
	// -------------------------------------------------------------------------
	ccTarget := mockTargetThatInstalls(t, ctrl, "claude-code", "Claude Code", installDirCC)

	_, err := install.NewWithTargets([]target.Target{ccTarget}).Run(ctx, install.Options{
		Source: archivePath,
		Skills: []string{"kubernetes-specialist"},
		Dir:    installDirCC,
	})
	require.NoError(t, err)

	// -------------------------------------------------------------------------
	// Step 3: Assert registry after first install.
	// -------------------------------------------------------------------------
	m, err := reg.Load("lifecycle-pkg")
	require.NoError(t, err)
	require.NotNil(t, m)

	assert.Equal(t, "lifecycle-pkg", m.Name)
	assert.Equal(t, []string{"kubernetes-specialist"}, m.SelectedSkills)
	assert.Equal(t, registry.ScopeLocal, m.Scope)

	// All files must be tagged to claude-code (first install only).
	for _, f := range m.Files {
		assert.Equal(t, "claude-code", f.Target)
	}

	// The mock target copies ALL content from the archive; SelectedSkills
	// records the user's intent but does not restrict which files are written
	// when installing from a pre-built archive (only git sources pre-filter).
	assertFilePath(t, m.Files, "skills/kubernetes-specialist/SKILL.md")
	assertFilePath(t, m.Files, "skills/react-expert/SKILL.md")

	// Commands and hooks also present (mock installs all non-.agentpack content).
	assertFilePath(t, m.Files, "commands/scan.md")
	assertFilePath(t, m.Files, "hooks/hooks.json")

	// -------------------------------------------------------------------------
	// Step 4: Install @react-expert to cursor under the same package name.
	// -------------------------------------------------------------------------
	cursorTarget := mockTargetThatInstalls(t, ctrl, "cursor", "Cursor", installDirCursor)

	_, err = install.NewWithTargets([]target.Target{cursorTarget}).Run(ctx, install.Options{
		Source: archivePath,
		Skills: []string{"react-expert"},
		Dir:    installDirCursor,
	})
	require.NoError(t, err)

	// -------------------------------------------------------------------------
	// Step 5: Assert registry MERGED -- single entry, both skills, both targets.
	// -------------------------------------------------------------------------
	m, err = reg.Load("lifecycle-pkg")
	require.NoError(t, err)
	require.NotNil(t, m)

	assert.ElementsMatch(t, []string{"kubernetes-specialist", "react-expert"}, m.SelectedSkills)

	targetNames := collectTargetNames(m.Files)
	assert.Contains(t, targetNames, "claude-code")
	assert.Contains(t, targetNames, "cursor")

	assertFilePath(t, m.Files, "skills/kubernetes-specialist/SKILL.md")
	assertFilePath(t, m.Files, "skills/react-expert/SKILL.md")

	// -------------------------------------------------------------------------
	// Step 6: List -- verify registry.List returns exactly one manifest with
	// both targets and both skills. We query the registry directly rather than
	// through pkg/list (which imports pkg/target/agents and would register the
	// universal target, breaking the "no targets" subtest).
	// -------------------------------------------------------------------------
	allManifests, listErr := reg.List()
	require.NoError(t, listErr)
	require.Len(t, allManifests, 1)

	listedM := allManifests[0]
	assert.Equal(t, "lifecycle-pkg", listedM.Name)

	assert.ElementsMatch(
		t,
		[]string{"kubernetes-specialist", "react-expert"},
		listedM.SelectedSkills,
	)

	listedTargets := collectTargetNames(listedM.Files)
	assert.ElementsMatch(t, []string{"claude-code", "cursor"}, listedTargets)

	// -------------------------------------------------------------------------
	// Step 7: Partial delete -- remove @react-expert only.
	// -------------------------------------------------------------------------
	_, err = remove.New().Run(ctx, remove.Options{
		Name:     "lifecycle-pkg",
		Skill:    "react-expert",
		Registry: reg,
	})
	require.NoError(t, err)

	// -------------------------------------------------------------------------
	// Step 8: Assert registry PRUNED -- entry survives, react-expert file gone.
	// -------------------------------------------------------------------------
	m, err = reg.Load("lifecycle-pkg")
	require.NoError(t, err)
	require.NotNil(t, m)

	assertNoFilePath(t, m.Files, "skills/react-expert/SKILL.md")
	assertFilePath(t, m.Files, "skills/kubernetes-specialist/SKILL.md")

	// react-expert SKILL.md must be removed from cursor's install directory.
	_, statErr := os.Stat(filepath.Join(installDirCursor, "skills", "react-expert", "SKILL.md"))
	assert.True(t, os.IsNotExist(statErr))

	// kubernetes-specialist SKILL.md must still exist in claude-code's install dir.
	_, statErr = os.Stat(filepath.Join(installDirCC, "skills", "kubernetes-specialist", "SKILL.md"))
	assert.NoError(t, statErr)

	// -------------------------------------------------------------------------
	// Step 9: Full delete.
	// -------------------------------------------------------------------------
	_, err = remove.New().Run(ctx, remove.Options{
		Name:     "lifecycle-pkg",
		Registry: reg,
	})
	require.NoError(t, err)

	// -------------------------------------------------------------------------
	// Step 10: Assert registry EMPTY -- manifest removed.
	// -------------------------------------------------------------------------
	_, loadErr := reg.Load("lifecycle-pkg")
	require.Error(t, loadErr)
	assert.Contains(t, loadErr.Error(), "not found in registry")
}

// testLifecycleWholeRepo installs without a Skills filter and checks that all
// content types are present in the registry and SelectedSkills is empty.
func testLifecycleWholeRepo(t *testing.T) {
	t.Helper()

	archivePath := lifecycleArchive(t, "whole-repo-pkg", "2.0.0")
	installDir := t.TempDir()

	reg, restoreHome := withTempRegistry(t)
	defer restoreHome()

	restoreSave := install.SetRegistrySave(reg.Save)
	defer restoreSave()

	restoreLoad := install.SetRegistryLoad(reg.Load)
	defer restoreLoad()

	ctx := context.Background()
	ctrl := gomock.NewController(t)

	tgt := mockTargetThatInstalls(t, ctrl, "claude-code", "Claude Code", installDir)

	_, err := install.NewWithTargets([]target.Target{tgt}).Run(ctx, install.Options{
		Source: archivePath,
		Skills: nil, // no filter -- install everything
		Dir:    installDir,
	})
	require.NoError(t, err)

	m, err := reg.Load("whole-repo-pkg")
	require.NoError(t, err)
	require.NotNil(t, m)

	// No explicit skill filter -> SelectedSkills must be empty.
	assert.Empty(t, m.SelectedSkills)

	// All content types present.
	assertFilePath(t, m.Files, "skills/kubernetes-specialist/SKILL.md")
	assertFilePath(t, m.Files, "skills/react-expert/SKILL.md")
	assertFilePath(t, m.Files, "commands/scan.md")
	assertFilePath(t, m.Files, "hooks/hooks.json")
}

// testLifecycleNoTargets asserts that when Targets is nil and target.Detected
// returns an empty slice the install pipeline returns the expected error.
//
// NOTE: Because this file imports the list package -- which transitively imports
// pkg/target/agents (registering the universal target that always detects) --
// target.Detected() will always return at least one entry in this test binary.
// The test therefore exercises the error path by passing a mock target whose
// Install always returns an error, verifying that the pipeline handles target
// failures correctly. The pure "no registered targets" scenario is covered by
// the parallel table in install_test.go ("errors with no targets (empty list)").
func testLifecycleNoTargets(t *testing.T) {
	t.Helper()

	archivePath := lifecycleArchive(t, "notarget-pkg", "1.0.0")
	ctx := context.Background()
	ctrl := gomock.NewController(t)

	// Provide exactly one failing target to simulate a target-level error
	// without depending on the ambient target registry.
	failTarget := mocks.NewMockTarget(ctrl)
	failTarget.EXPECT().Name().Return("fail-target").AnyTimes()
	failTarget.EXPECT().DisplayName().Return("Fail Target").AnyTimes()
	failTarget.EXPECT().
		SupportedTypes().
		Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
		AnyTimes()
	failTarget.EXPECT().
		Install(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("simulated: no agent available"))

	_, err := install.NewWithTargets([]target.Target{failTarget}).Run(ctx, install.Options{
		Source: archivePath,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "install to fail-target")
}

// testLifecycleSkillFilter exercises ADR-008: when Skills=["kubernetes-specialist"]
// is set, only skills/ content is recorded in the registry (the mock target
// installs everything from the archive, but SelectedSkills signals intent and
// the registry manifest reflects what was tracked). This test verifies that
// registry.SelectedSkills is set correctly and that only the requested skill
// appears in SelectedSkills while unfiltered commands/hooks remain absent from
// that field.
func testLifecycleSkillFilter(t *testing.T) {
	t.Helper()

	archivePath := lifecycleArchive(t, "skill-filter-pkg", "1.0.0")
	installDir := t.TempDir()

	reg, restoreHome := withTempRegistry(t)
	defer restoreHome()

	restoreSave := install.SetRegistrySave(reg.Save)
	defer restoreSave()

	restoreLoad := install.SetRegistryLoad(reg.Load)
	defer restoreLoad()

	ctx := context.Background()
	ctrl := gomock.NewController(t)

	// skillTarget only writes files whose path contains "kubernetes-specialist"
	// to simulate the ADR-008 server-side filtering that a real target would do.
	m := mocks.NewMockTarget(ctrl)
	m.EXPECT().Name().Return("claude-code").AnyTimes()
	m.EXPECT().DisplayName().Return("Claude Code").AnyTimes()
	m.EXPECT().
		SupportedTypes().
		Return([]string{"skill", "command", "hook", "agent", "mcp", "config"}).
		AnyTimes()
	m.EXPECT().
		Install(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, opts target.InstallOpts) ([]target.InstalledFile, error) {
			var installed []target.InstalledFile

			err := filepath.WalkDir(
				opts.SourceDir,
				func(path string, d os.DirEntry, walkErr error) error {
					if walkErr != nil {
						return walkErr
					}

					if d.IsDir() {
						return nil
					}

					rel, relErr := filepath.Rel(opts.SourceDir, path)
					if relErr != nil {
						return relErr
					}

					// Skip .agentpack metadata files.
					if len(rel) >= len(".agentpack") && rel[:len(".agentpack")] == ".agentpack" {
						return nil
					}

					// Only install files that belong to the requested skill.
					normalized := filepath.ToSlash(rel)
					if !strings.Contains(normalized, "kubernetes-specialist") {
						return nil
					}

					dst := filepath.Join(installDir, rel)
					if mkErr := os.MkdirAll(filepath.Dir(dst), 0o755); mkErr != nil {
						return mkErr
					}

					data, readErr := os.ReadFile(path)
					if readErr != nil {
						return readErr
					}

					if writeErr := os.WriteFile(dst, data, 0o644); writeErr != nil {
						return writeErr
					}

					h := sha256.Sum256(data)
					installed = append(installed, target.InstalledFile{
						Path:   rel,
						SHA256: hex.EncodeToString(h[:]),
					})

					return nil
				},
			)

			return installed, err
		}).
		AnyTimes()

	_, err := install.NewWithTargets([]target.Target{m}).Run(ctx, install.Options{
		Source: archivePath,
		Skills: []string{"kubernetes-specialist"},
		Dir:    installDir,
	})
	require.NoError(t, err)

	manifest, loadErr := reg.Load("skill-filter-pkg")
	require.NoError(t, loadErr)
	require.NotNil(t, manifest)

	// SelectedSkills must record the user's intent.
	assert.Equal(t, []string{"kubernetes-specialist"}, manifest.SelectedSkills)

	// Only the kubernetes-specialist skill file must appear in the registry
	// -- the skill-filtering mock target only installed that file.
	assertFilePath(t, manifest.Files, "skills/kubernetes-specialist/SKILL.md")
	assertNoFilePath(t, manifest.Files, "skills/react-expert/SKILL.md")
	assertNoFilePath(t, manifest.Files, "commands/scan.md")
	assertNoFilePath(t, manifest.Files, "hooks/hooks.json")
}

// testLifecycleWholeRepoAllContent installs without a Skills filter and
// verifies that skills, commands, and hooks all appear in the registry manifest.
func testLifecycleWholeRepoAllContent(t *testing.T) {
	t.Helper()

	archivePath := lifecycleArchive(t, "all-content-pkg", "1.0.0")
	installDir := t.TempDir()

	reg, restoreHome := withTempRegistry(t)
	defer restoreHome()

	restoreSave := install.SetRegistrySave(reg.Save)
	defer restoreSave()

	restoreLoad := install.SetRegistryLoad(reg.Load)
	defer restoreLoad()

	ctx := context.Background()
	ctrl := gomock.NewController(t)

	tgt := mockTargetThatInstalls(t, ctrl, "claude-code", "Claude Code", installDir)

	_, err := install.NewWithTargets([]target.Target{tgt}).Run(ctx, install.Options{
		Source: archivePath,
		Skills: nil, // whole repo -- no filter
		Dir:    installDir,
	})
	require.NoError(t, err)

	manifest, loadErr := reg.Load("all-content-pkg")
	require.NoError(t, loadErr)
	require.NotNil(t, manifest)

	// No explicit skill filter -> SelectedSkills must be nil/empty.
	assert.Empty(t, manifest.SelectedSkills)

	// All content types from the archive must appear in the registry.
	assertFilePath(t, manifest.Files, "skills/kubernetes-specialist/SKILL.md")
	assertFilePath(t, manifest.Files, "skills/react-expert/SKILL.md")
	assertFilePath(t, manifest.Files, "commands/scan.md")
	assertFilePath(t, manifest.Files, "hooks/hooks.json")
}

// testLifecycleSkillThenWholeRepo installs a single skill to target A, then
// installs the whole repo to target B. It verifies that SelectedSkills merges
// correctly and that each target's files appear in the registry.
func testLifecycleSkillThenWholeRepo(t *testing.T) {
	t.Helper()

	archivePath := lifecycleArchive(t, "merge-pkg", "1.0.0")
	installDirA := t.TempDir()
	installDirB := t.TempDir()

	reg, restoreHome := withTempRegistry(t)
	defer restoreHome()

	restoreSave := install.SetRegistrySave(reg.Save)
	defer restoreSave()

	restoreLoad := install.SetRegistryLoad(reg.Load)
	defer restoreLoad()

	ctx := context.Background()
	ctrl := gomock.NewController(t)

	// -------------------------------------------------------------------------
	// Step 1: Install @kubernetes-specialist to target A.
	// -------------------------------------------------------------------------
	targetA := mockTargetThatInstalls(t, ctrl, "claude-code", "Claude Code", installDirA)

	_, err := install.NewWithTargets([]target.Target{targetA}).Run(ctx, install.Options{
		Source: archivePath,
		Skills: []string{"kubernetes-specialist"},
		Dir:    installDirA,
	})
	require.NoError(t, err)

	// -------------------------------------------------------------------------
	// Step 2: Install whole repo to target B.
	// -------------------------------------------------------------------------
	targetB := mockTargetThatInstalls(t, ctrl, "cursor", "Cursor", installDirB)

	_, err = install.NewWithTargets([]target.Target{targetB}).Run(ctx, install.Options{
		Source: archivePath,
		Skills: nil, // no filter
		Dir:    installDirB,
	})
	require.NoError(t, err)

	// -------------------------------------------------------------------------
	// Step 3: Assert merged registry.
	// -------------------------------------------------------------------------
	manifest, loadErr := reg.Load("merge-pkg")
	require.NoError(t, loadErr)
	require.NotNil(t, manifest)

	// SelectedSkills must contain the skill from the first install; the second
	// install (nil Skills) contributes no additional entries.
	assert.Contains(t, manifest.SelectedSkills, "kubernetes-specialist")

	// Both targets must appear in the file list.
	targetNames := collectTargetNames(manifest.Files)
	assert.Contains(t, targetNames, "claude-code")
	assert.Contains(t, targetNames, "cursor")

	// Target A (claude-code) files: mock installs all archive content.
	var ccFiles []registry.InstalledFile
	for _, f := range manifest.Files {
		if f.Target == "claude-code" {
			ccFiles = append(ccFiles, f)
		}
	}

	assertFilePath(t, ccFiles, "skills/kubernetes-specialist/SKILL.md")

	// Target B (cursor) files: whole-repo install includes all content.
	var cursorFiles []registry.InstalledFile
	for _, f := range manifest.Files {
		if f.Target == "cursor" {
			cursorFiles = append(cursorFiles, f)
		}
	}

	assertFilePath(t, cursorFiles, "skills/kubernetes-specialist/SKILL.md")
	assertFilePath(t, cursorFiles, "skills/react-expert/SKILL.md")
	assertFilePath(t, cursorFiles, "commands/scan.md")
	assertFilePath(t, cursorFiles, "hooks/hooks.json")
}

// --------------------------------------------------------------------------
// TestLifecycleManifests
// --------------------------------------------------------------------------

// TestLifecycleManifests exercises the full manifest pipeline: yaml, lock,
// and registry are all written and asserted after each add/merge/del step.
// This mirrors what cmd/add and cmd/del do end-to-end.
func TestLifecycleManifests(t *testing.T) {
	tests := []struct {
		name       string
		noParallel bool
		run        func(t *testing.T)
	}{
		{
			name:       "add writes yaml + lock + registry, merge accumulates, del prunes, full del cleans",
			noParallel: true,
			run:        testManifestFullLifecycle,
		},
		{
			name:       "two packages: independent yaml + lock entries, remove one leaves the other",
			noParallel: true,
			run:        testManifestTwoPackages,
		},
		{
			name:       "whole repo add: yaml has no skills, lock has no skills, registry has all content",
			noParallel: true,
			run:        testManifestWholeRepo,
		},
		{
			name:       "SupportedTypes filtering: skill-only target receives only skill entries",
			noParallel: true,
			run:        testManifestSupportedTypesFiltering,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.noParallel {
				t.Parallel()
			}

			tt.run(t)
		})
	}
}

// manifestEnv bundles the temp paths and registry for manifest tests.
type manifestEnv struct {
	reg      *registry.Registry
	yamlPath string
	lockPath string
}

// newManifestEnv creates a temp sandbox with redirected registry, yaml, and
// lock paths. Callers must defer the returned restore function.
func newManifestEnv(t *testing.T) (*manifestEnv, func()) {
	t.Helper()

	sandboxDir := t.TempDir()

	reg, restoreHome := withTempRegistry(t)
	restoreSave := install.SetRegistrySave(reg.Save)
	restoreLoad := install.SetRegistryLoad(reg.Load)

	restore := func() {
		restoreLoad()
		restoreSave()
		restoreHome()
	}

	return &manifestEnv{
		reg:      reg,
		yamlPath: filepath.Join(sandboxDir, "agentpack-packages.yaml"),
		lockPath: filepath.Join(sandboxDir, "agentpack.lock"),
	}, restore
}

// simulateAddManifests mirrors what cmd/add.updateManifests does: writes the
// installed package into the yaml spec and lock file.
func simulateAddManifests(
	env *manifestEnv,
	name, source string,
	skills, targets []string,
	sha string,
) error {
	cfg, err := packages.Load(env.yamlPath)
	if err != nil {
		return err
	}

	pkg := packages.Package{
		Name: name,
		Git:  source,
	}
	if len(skills) > 0 {
		pkg.Skills = skills
	}
	if len(targets) > 0 {
		pkg.Targets = targets
	}

	cfg.Add(pkg)

	if err := packages.Save(env.yamlPath, cfg); err != nil {
		return err
	}

	lf, err := lock.Load(env.lockPath)
	if err != nil {
		return err
	}

	lp := lock.LockedPackage{
		Name:     name,
		Source:   source,
		SHA:      sha,
		Resolved: "2026-01-01T00:00:00Z",
		Skills:   skills,
		Targets:  targets,
	}

	lf.Set(lp)

	return lock.Save(env.lockPath, lf)
}

// simulateDelManifests mirrors what cmd/del.removeManifests does.
func simulateDelManifests(env *manifestEnv, name, skill string) {
	if skill != "" {
		if cfg, err := packages.Load(env.yamlPath); err == nil {
			if p := cfg.Find(name); p != nil {
				remaining := make([]string, 0, len(p.Skills))
				for _, s := range p.Skills {
					if s != skill {
						remaining = append(remaining, s)
					}
				}
				p.Skills = remaining
			}

			_ = packages.Save(env.yamlPath, cfg)
		}

		if lf, err := lock.Load(env.lockPath); err == nil {
			lf.RemoveSkill(name, skill)
			_ = lock.Save(env.lockPath, lf)
		}

		return
	}

	if cfg, err := packages.Load(env.yamlPath); err == nil {
		cfg.Remove(name)
		_ = packages.Save(env.yamlPath, cfg)
	}

	if lf, err := lock.Load(env.lockPath); err == nil {
		lf.Remove(name)
		_ = lock.Save(env.lockPath, lf)
	}
}

// testManifestFullLifecycle exercises:
//  1. Add @kubernetes-specialist to claude-code -> assert yaml/lock/registry
//  2. Add @react-expert to cursor (same pkg) -> assert merge in all three
//  3. Partial del @react-expert -> assert yaml/lock pruned, registry pruned
//  4. Full del -> assert yaml/lock/registry all empty
func testManifestFullLifecycle(t *testing.T) {
	t.Helper()

	archivePath := lifecycleArchive(t, "manifest-pkg", "1.0.0")
	installDirCC := t.TempDir()
	installDirCursor := t.TempDir()

	env, restore := newManifestEnv(t)
	defer restore()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	sha := "deadbeef1234567890abcdef"

	// -------------------------------------------------------------------------
	// Step 1: Add @kubernetes-specialist -> claude-code
	// -------------------------------------------------------------------------
	ccTarget := mockTargetThatInstalls(t, ctrl, "claude-code", "Claude Code", installDirCC)

	_, err := install.NewWithTargets([]target.Target{ccTarget}).Run(ctx, install.Options{
		Source: archivePath,
		Skills: []string{"kubernetes-specialist"},
		Dir:    installDirCC,
	})
	require.NoError(t, err)

	require.NoError(t, simulateAddManifests(
		env, "manifest-pkg", "github.com/test/manifest-pkg",
		[]string{"kubernetes-specialist"}, []string{"claude-code"}, sha,
	))

	// Assert yaml.
	cfg, err := packages.Load(env.yamlPath)
	require.NoError(t, err)
	require.Len(t, cfg.Packages, 1)
	assert.Equal(t, "manifest-pkg", cfg.Packages[0].Name)
	assert.Equal(t, []string{"kubernetes-specialist"}, cfg.Packages[0].Skills)
	assert.Equal(t, []string{"claude-code"}, cfg.Packages[0].Targets)

	// Assert lock.
	lf, err := lock.Load(env.lockPath)
	require.NoError(t, err)
	require.Len(t, lf.Packages, 1)
	assert.Equal(t, "manifest-pkg", lf.Packages[0].Name)
	assert.Equal(t, sha, lf.Packages[0].SHA)
	assert.Equal(t, []string{"kubernetes-specialist"}, lf.Packages[0].Skills)

	// Assert registry.
	m, err := env.reg.Load("manifest-pkg")
	require.NoError(t, err)
	assert.Equal(t, []string{"kubernetes-specialist"}, m.SelectedSkills)
	assertFilePath(t, m.Files, "skills/kubernetes-specialist/SKILL.md")

	// -------------------------------------------------------------------------
	// Step 2: Add @react-expert -> cursor (merge)
	// -------------------------------------------------------------------------
	cursorTarget := mockTargetThatInstalls(t, ctrl, "cursor", "Cursor", installDirCursor)

	_, err = install.NewWithTargets([]target.Target{cursorTarget}).Run(ctx, install.Options{
		Source: archivePath,
		Skills: []string{"react-expert"},
		Dir:    installDirCursor,
	})
	require.NoError(t, err)

	require.NoError(t, simulateAddManifests(
		env, "manifest-pkg", "github.com/test/manifest-pkg",
		[]string{"react-expert"}, []string{"cursor"}, sha,
	))

	// Assert yaml merged.
	cfg, err = packages.Load(env.yamlPath)
	require.NoError(t, err)
	require.Len(t, cfg.Packages, 1)
	assert.ElementsMatch(t, []string{"claude-code", "cursor"}, cfg.Packages[0].Targets)
	assert.ElementsMatch(
		t,
		[]string{"kubernetes-specialist", "react-expert"},
		cfg.Packages[0].Skills,
	)

	// Assert lock merged.
	lf, err = lock.Load(env.lockPath)
	require.NoError(t, err)
	require.Len(t, lf.Packages, 1)
	assert.ElementsMatch(
		t,
		[]string{"kubernetes-specialist", "react-expert"},
		lf.Packages[0].Skills,
	)
	assert.ElementsMatch(t, []string{"claude-code", "cursor"}, lf.Packages[0].Targets)

	// Assert registry merged.
	m, err = env.reg.Load("manifest-pkg")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"kubernetes-specialist", "react-expert"}, m.SelectedSkills)

	targetNames := collectTargetNames(m.Files)
	assert.Contains(t, targetNames, "claude-code")
	assert.Contains(t, targetNames, "cursor")

	// -------------------------------------------------------------------------
	// Step 3: Partial del @react-expert
	// -------------------------------------------------------------------------
	_, err = remove.New().Run(ctx, remove.Options{
		Name:     "manifest-pkg",
		Skill:    "react-expert",
		Registry: env.reg,
	})
	require.NoError(t, err)

	simulateDelManifests(env, "manifest-pkg", "react-expert")

	// Assert yaml pruned.
	cfg, err = packages.Load(env.yamlPath)
	require.NoError(t, err)
	require.Len(t, cfg.Packages, 1)
	assert.Equal(t, []string{"kubernetes-specialist"}, cfg.Packages[0].Skills)

	// Assert lock pruned.
	lf, err = lock.Load(env.lockPath)
	require.NoError(t, err)
	require.Len(t, lf.Packages, 1)
	lp := lf.Find("manifest-pkg")
	require.NotNil(t, lp)
	assert.Equal(t, []string{"kubernetes-specialist"}, lp.Skills)

	// Assert registry pruned.
	m, err = env.reg.Load("manifest-pkg")
	require.NoError(t, err)
	assertNoFilePath(t, m.Files, "skills/react-expert/SKILL.md")
	assertFilePath(t, m.Files, "skills/kubernetes-specialist/SKILL.md")

	// -------------------------------------------------------------------------
	// Step 4: Full del
	// -------------------------------------------------------------------------
	_, err = remove.New().Run(ctx, remove.Options{
		Name:     "manifest-pkg",
		Registry: env.reg,
	})
	require.NoError(t, err)

	simulateDelManifests(env, "manifest-pkg", "")

	// Assert yaml empty.
	cfg, err = packages.Load(env.yamlPath)
	require.NoError(t, err)
	assert.Empty(t, cfg.Packages)

	// Assert lock empty.
	lf, err = lock.Load(env.lockPath)
	require.NoError(t, err)
	assert.Empty(t, lf.Packages)

	// Assert registry gone.
	_, loadErr := env.reg.Load("manifest-pkg")
	require.Error(t, loadErr)
	assert.Contains(t, loadErr.Error(), "not found")
}

// testManifestTwoPackages installs two independent packages and verifies
// that removing one leaves the other intact across yaml, lock, and registry.
func testManifestTwoPackages(t *testing.T) {
	t.Helper()

	archiveA := lifecycleArchive(t, "pkg-alpha", "1.0.0")
	archiveB := lifecycleArchive(t, "pkg-beta", "2.0.0")
	installDirA := t.TempDir()
	installDirB := t.TempDir()

	env, restore := newManifestEnv(t)
	defer restore()

	ctx := context.Background()
	ctrl := gomock.NewController(t)

	// -------------------------------------------------------------------------
	// Install pkg-alpha
	// -------------------------------------------------------------------------
	targetA := mockTargetThatInstalls(t, ctrl, "claude-code", "Claude Code", installDirA)

	_, err := install.NewWithTargets([]target.Target{targetA}).Run(ctx, install.Options{
		Source: archiveA,
		Skills: []string{"kubernetes-specialist"},
		Dir:    installDirA,
	})
	require.NoError(t, err)

	require.NoError(t, simulateAddManifests(
		env, "pkg-alpha", "github.com/test/pkg-alpha",
		[]string{"kubernetes-specialist"}, []string{"claude-code"}, "sha-alpha",
	))

	// -------------------------------------------------------------------------
	// Install pkg-beta
	// -------------------------------------------------------------------------
	targetB := mockTargetThatInstalls(t, ctrl, "cursor", "Cursor", installDirB)

	_, err = install.NewWithTargets([]target.Target{targetB}).Run(ctx, install.Options{
		Source: archiveB,
		Dir:    installDirB,
	})
	require.NoError(t, err)

	require.NoError(t, simulateAddManifests(
		env, "pkg-beta", "github.com/test/pkg-beta",
		nil, []string{"cursor"}, "sha-beta",
	))

	// Assert both in yaml.
	cfg, err := packages.Load(env.yamlPath)
	require.NoError(t, err)
	require.Len(t, cfg.Packages, 2)
	assert.Equal(t, "pkg-alpha", cfg.Packages[0].Name)
	assert.Equal(t, "pkg-beta", cfg.Packages[1].Name)

	// Assert both in lock.
	lf, err := lock.Load(env.lockPath)
	require.NoError(t, err)
	require.Len(t, lf.Packages, 2)

	// Assert both in registry.
	allManifests, err := env.reg.List()
	require.NoError(t, err)
	require.Len(t, allManifests, 2)

	// -------------------------------------------------------------------------
	// Remove pkg-alpha -- pkg-beta survives
	// -------------------------------------------------------------------------
	_, err = remove.New().Run(ctx, remove.Options{
		Name:     "pkg-alpha",
		Registry: env.reg,
	})
	require.NoError(t, err)

	simulateDelManifests(env, "pkg-alpha", "")

	// Assert yaml has only pkg-beta.
	cfg, err = packages.Load(env.yamlPath)
	require.NoError(t, err)
	require.Len(t, cfg.Packages, 1)
	assert.Equal(t, "pkg-beta", cfg.Packages[0].Name)

	// Assert lock has only pkg-beta.
	lf, err = lock.Load(env.lockPath)
	require.NoError(t, err)
	require.Len(t, lf.Packages, 1)
	assert.Equal(t, "pkg-beta", lf.Packages[0].Name)

	// Assert registry has only pkg-beta.
	allManifests, err = env.reg.List()
	require.NoError(t, err)
	require.Len(t, allManifests, 1)
	assert.Equal(t, "pkg-beta", allManifests[0].Name)
}

// testManifestSupportedTypesFiltering verifies ADR-009: the install pipeline
// filters opts.Entries against each target's SupportedTypes before calling
// Install. An all-types target receives every entry; a skill-only target
// receives only skill entries.
func testManifestSupportedTypesFiltering(t *testing.T) {
	t.Helper()

	archivePath := lifecycleArchive(t, "filter-types-pkg", "1.0.0")
	installDirAll := t.TempDir()
	installDirSkillOnly := t.TempDir()

	env, restore := newManifestEnv(t)
	defer restore()

	ctx := context.Background()
	ctrl := gomock.NewController(t)

	// allTypesTarget accepts every content type -- the baseline.
	allTypesTarget := mockTargetThatInstalls(t, ctrl, "claude-code", "Claude Code", installDirAll)

	// skillOnlyTarget declares support for "skill" only. We use DoAndReturn so
	// we can capture and assert on the Entries the pipeline passes in.
	var capturedEntries []target.ContentEntry

	skillOnlyTarget := mocks.NewMockTarget(ctrl)
	skillOnlyTarget.EXPECT().Name().Return("skill-only").AnyTimes()
	skillOnlyTarget.EXPECT().DisplayName().Return("Skill Only").AnyTimes()
	skillOnlyTarget.EXPECT().
		SupportedTypes().
		Return([]string{"skill"}).
		AnyTimes()
	skillOnlyTarget.EXPECT().
		Install(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, opts target.InstallOpts) ([]target.InstalledFile, error) {
			capturedEntries = opts.Entries

			var installed []target.InstalledFile

			err := filepath.WalkDir(
				opts.SourceDir,
				func(path string, d os.DirEntry, walkErr error) error {
					if walkErr != nil {
						return walkErr
					}

					if d.IsDir() {
						return nil
					}

					rel, relErr := filepath.Rel(opts.SourceDir, path)
					if relErr != nil {
						return relErr
					}

					// Skip .agentpack metadata files.
					if len(rel) >= len(".agentpack") && rel[:len(".agentpack")] == ".agentpack" {
						return nil
					}

					// Only install files under a skills/ subtree.
					normalized := filepath.ToSlash(rel)
					if !strings.HasPrefix(normalized, "skills/") {
						return nil
					}

					dst := filepath.Join(installDirSkillOnly, rel)
					if mkErr := os.MkdirAll(filepath.Dir(dst), 0o755); mkErr != nil {
						return mkErr
					}

					data, readErr := os.ReadFile(path)
					if readErr != nil {
						return readErr
					}

					if writeErr := os.WriteFile(dst, data, 0o644); writeErr != nil {
						return writeErr
					}

					h := sha256.Sum256(data)
					installed = append(installed, target.InstalledFile{
						Path:   rel,
						SHA256: hex.EncodeToString(h[:]),
					})

					return nil
				},
			)

			return installed, err
		}).
		AnyTimes()

	_, err := install.NewWithTargets([]target.Target{allTypesTarget, skillOnlyTarget}).
		Run(ctx, install.Options{
			Source: archivePath,
			Dir:    installDirAll,
		})
	require.NoError(t, err)

	// The skill-only target must have received only skill-typed entries.
	require.NotEmpty(t, capturedEntries)

	for _, e := range capturedEntries {
		assert.Equal(t, "skill", e.Type)
	}

	// The all-types target's registry entries include commands and hooks.
	manifest, loadErr := env.reg.Load("filter-types-pkg")
	require.NoError(t, loadErr)
	require.NotNil(t, manifest)

	var allTypesFiles []registry.InstalledFile
	var skillOnlyFiles []registry.InstalledFile

	for _, f := range manifest.Files {
		switch f.Target {
		case "claude-code":
			allTypesFiles = append(allTypesFiles, f)
		case "skill-only":
			skillOnlyFiles = append(skillOnlyFiles, f)
		}
	}

	// All-types target must have received skills, commands, and hooks.
	assertFilePath(t, allTypesFiles, "skills/kubernetes-specialist/SKILL.md")
	assertFilePath(t, allTypesFiles, "skills/react-expert/SKILL.md")
	assertFilePath(t, allTypesFiles, "commands/scan.md")
	assertFilePath(t, allTypesFiles, "hooks/hooks.json")

	// Skill-only target must have skills but no commands or hooks.
	assertFilePath(t, skillOnlyFiles, "skills/kubernetes-specialist/SKILL.md")
	assertFilePath(t, skillOnlyFiles, "skills/react-expert/SKILL.md")
	assertNoFilePath(t, skillOnlyFiles, "commands/scan.md")
	assertNoFilePath(t, skillOnlyFiles, "hooks/hooks.json")
}

// testManifestWholeRepo installs a whole repo (no skill filter) and verifies
// that yaml has no skills field, lock has no skills field, and registry has
// all content types.
func testManifestWholeRepo(t *testing.T) {
	t.Helper()

	archivePath := lifecycleArchive(t, "whole-pkg", "3.0.0")
	installDir := t.TempDir()

	env, restore := newManifestEnv(t)
	defer restore()

	ctx := context.Background()
	ctrl := gomock.NewController(t)

	tgt := mockTargetThatInstalls(t, ctrl, "claude-code", "Claude Code", installDir)

	_, err := install.NewWithTargets([]target.Target{tgt}).Run(ctx, install.Options{
		Source: archivePath,
		Dir:    installDir,
	})
	require.NoError(t, err)

	require.NoError(t, simulateAddManifests(
		env, "whole-pkg", "github.com/test/whole-pkg",
		nil, []string{"claude-code"}, "sha-whole",
	))

	// Assert yaml -- no skills filter.
	cfg, err := packages.Load(env.yamlPath)
	require.NoError(t, err)
	require.Len(t, cfg.Packages, 1)
	assert.Empty(t, cfg.Packages[0].Skills)
	assert.Equal(t, []string{"claude-code"}, cfg.Packages[0].Targets)

	// Assert lock -- no skills.
	lf, err := lock.Load(env.lockPath)
	require.NoError(t, err)
	require.Len(t, lf.Packages, 1)
	assert.Empty(t, lf.Packages[0].Skills)

	// Assert registry -- all content types present.
	m, err := env.reg.Load("whole-pkg")
	require.NoError(t, err)
	assert.Empty(t, m.SelectedSkills)
	assertFilePath(t, m.Files, "skills/kubernetes-specialist/SKILL.md")
	assertFilePath(t, m.Files, "skills/react-expert/SKILL.md")
	assertFilePath(t, m.Files, "commands/scan.md")
	assertFilePath(t, m.Files, "hooks/hooks.json")
}
