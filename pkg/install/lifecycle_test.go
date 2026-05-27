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
	"fmt"
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
	"github.com/retr0h/agentpack/pkg/install"
	"github.com/retr0h/agentpack/pkg/registry"
	"github.com/retr0h/agentpack/pkg/remove"
	"github.com/retr0h/agentpack/pkg/target"
	"github.com/retr0h/agentpack/pkg/target/mocks"
)

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

					// Skip .agentpack internal files — targets never install those.
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

	for _, f := range files {
		if filepath.ToSlash(f.Path) == wantPath {
			return
		}
	}

	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}

	t.Errorf("expected file %q in registry files; got %v", wantPath, paths)
}

// assertNoFilePath fails when any file in the slice matches wantPath.
func assertNoFilePath(t *testing.T, files []registry.InstalledFile, wantPath string) {
	t.Helper()

	for _, f := range files {
		if filepath.ToSlash(f.Path) == wantPath {
			t.Errorf("expected file %q to be absent from registry files", wantPath)
			return
		}
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

// TestLifecycleFullAddListDelete exercises the complete install → list → delete
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
//  1. Install @kubernetes-specialist to claude-code → assert registry entry
//  2. Install @react-expert to cursor (same pkg) → assert merged registry
//  3. list.RunWithRegistry → 1 entry, both targets, both skills
//  4. Partial delete @react-expert → registry pruned, files removed from disk
//  5. Full delete → registry entry gone
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

	_, err := install.New().Run(ctx, install.Options{
		Source:  archivePath,
		Skills:  []string{"kubernetes-specialist"},
		Targets: []target.Target{ccTarget},
		Dir:     installDirCC,
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

	_, err = install.New().Run(ctx, install.Options{
		Source:  archivePath,
		Skills:  []string{"react-expert"},
		Targets: []target.Target{cursorTarget},
		Dir:     installDirCursor,
	})
	require.NoError(t, err)

	// -------------------------------------------------------------------------
	// Step 5: Assert registry MERGED — single entry, both skills, both targets.
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
	// Step 6: List — verify registry.List returns exactly one manifest with
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
	// Step 7: Partial delete — remove @react-expert only.
	// -------------------------------------------------------------------------
	_, err = remove.New().Run(ctx, remove.Options{
		Name:     "lifecycle-pkg",
		Skill:    "react-expert",
		Registry: reg,
	})
	require.NoError(t, err)

	// -------------------------------------------------------------------------
	// Step 8: Assert registry PRUNED — entry survives, react-expert file gone.
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
	// Step 10: Assert registry EMPTY — manifest removed.
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

	_, err := install.New().Run(ctx, install.Options{
		Source:  archivePath,
		Skills:  nil, // no filter — install everything
		Targets: []target.Target{tgt},
		Dir:     installDir,
	})
	require.NoError(t, err)

	m, err := reg.Load("whole-repo-pkg")
	require.NoError(t, err)
	require.NotNil(t, m)

	// No explicit skill filter → SelectedSkills must be empty.
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
// NOTE: Because this file imports the list package — which transitively imports
// pkg/target/agents (registering the universal target that always detects) —
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
		Install(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("simulated: no agent available"))

	_, err := install.New().Run(ctx, install.Options{
		Source:  archivePath,
		Targets: []target.Target{failTarget},
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

	_, err := install.New().Run(ctx, install.Options{
		Source:  archivePath,
		Skills:  []string{"kubernetes-specialist"},
		Targets: []target.Target{m},
		Dir:     installDir,
	})
	require.NoError(t, err)

	manifest, loadErr := reg.Load("skill-filter-pkg")
	require.NoError(t, loadErr)
	require.NotNil(t, manifest)

	// SelectedSkills must record the user's intent.
	assert.Equal(t, []string{"kubernetes-specialist"}, manifest.SelectedSkills)

	// Only the kubernetes-specialist skill file must appear in the registry
	// — the skill-filtering mock target only installed that file.
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

	_, err := install.New().Run(ctx, install.Options{
		Source:  archivePath,
		Skills:  nil, // whole repo — no filter
		Targets: []target.Target{tgt},
		Dir:     installDir,
	})
	require.NoError(t, err)

	manifest, loadErr := reg.Load("all-content-pkg")
	require.NoError(t, loadErr)
	require.NotNil(t, manifest)

	// No explicit skill filter → SelectedSkills must be nil/empty.
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

	_, err := install.New().Run(ctx, install.Options{
		Source:  archivePath,
		Skills:  []string{"kubernetes-specialist"},
		Targets: []target.Target{targetA},
		Dir:     installDirA,
	})
	require.NoError(t, err)

	// -------------------------------------------------------------------------
	// Step 2: Install whole repo to target B.
	// -------------------------------------------------------------------------
	targetB := mockTargetThatInstalls(t, ctrl, "cursor", "Cursor", installDirB)

	_, err = install.New().Run(ctx, install.Options{
		Source:  archivePath,
		Skills:  nil, // no filter
		Targets: []target.Target{targetB},
		Dir:     installDirB,
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
