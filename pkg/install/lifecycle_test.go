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

	fileEntries = append(fileEntries,
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

			err := filepath.WalkDir(opts.SourceDir, func(path string, d os.DirEntry, walkErr error) error {
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
			})

			return installed, err
		}).AnyTimes()

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

	assert.ElementsMatch(t, []string{"kubernetes-specialist", "react-expert"}, listedM.SelectedSkills)

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
