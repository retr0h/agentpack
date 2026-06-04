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

// Package install export_test.go exposes private helpers for white-box testing.
package install

import (
	"context"
	"errors"
	"os"

	"github.com/retr0h/agentpack/internal/archive"
	"github.com/retr0h/agentpack/internal/cli"
	"github.com/retr0h/agentpack/internal/fetcher"
	"github.com/retr0h/agentpack/internal/gitutil"
	"github.com/retr0h/agentpack/internal/metadata"
	"github.com/retr0h/agentpack/internal/target"
	"github.com/retr0h/agentpack/pkg/registry"
)

// StoreArchive exposes the storeArchive method for testing.
func (i *Installer) StoreArchive(ctx context.Context, srcPath, name, sha string) (string, error) {
	return i.storeArchive(ctx, srcPath, name, sha)
}

// FilteredSubdirs exposes filteredSubdirs for testing.
func FilteredSubdirs(contentDir string, filter []string) []string {
	return filteredSubdirs(contentDir, filter)
}

// ArchivesDir exposes the archivesDir seam for testing.
func (i *Installer) ArchivesDir() (string, error) {
	return i.archivesDir()
}

// SetArchivesDir replaces the installer's archives-dir resolver for testing.
func (i *Installer) SetArchivesDir(fn func() (string, error)) {
	i.archivesDir = fn
}

// ArchivesDirForHome exposes archivesDirForHome for testing the default
// archives-dir resolution against a controlled home directory.
func ArchivesDirForHome(homeFn func() (string, error)) (string, error) {
	return archivesDirForHome(homeFn)
}

// ShortSHA exposes gitutil.ShortSHA for testing.
func ShortSHA(sha string) string {
	return gitutil.ShortSHA(sha)
}

// FindChecksums exposes findChecksums for testing.
func FindChecksums(ctx context.Context, dir string) (string, error) {
	return findChecksums(ctx, dir)
}

// FindAndReadMetadata exposes findAndReadMetadata for testing.
func FindAndReadMetadata(ctx context.Context, dir string) (*metadata.Metadata, error) {
	return findAndReadMetadata(ctx, dir)
}

// SetCreateTemp replaces the installer's temp-file creator for testing.
func (i *Installer) SetCreateTemp(fn func(string, string) (*os.File, error)) {
	i.createTemp = fn
}

// SetMkdirTemp replaces the installer's temp-dir creator for testing.
func (i *Installer) SetMkdirTemp(fn func(string, string) (string, error)) {
	i.mkdirTemp = fn
}

// CreateTempAlwaysFails is a createTemp seam that always returns an error.
func CreateTempAlwaysFails(_, _ string) (*os.File, error) {
	return nil, errors.New("simulated create temp failure")
}

// MkdirTempAlwaysFails is a mkdirTemp seam that always returns an error.
func MkdirTempAlwaysFails(_, _ string) (string, error) {
	return "", errors.New("simulated mkdir temp failure")
}

// MkdirTempFailAfterN returns a mkdirTemp seam that succeeds on the first n
// calls and then returns an error.
func MkdirTempFailAfterN(n int) func(string, string) (string, error) {
	call := 0
	return func(dir, pattern string) (string, error) {
		call++
		if call > n {
			return "", errors.New("simulated mkdir temp failure after n")
		}
		return os.MkdirTemp(dir, pattern)
	}
}

// CopyToTemp exposes the copyToTemp method for testing.
func (i *Installer) CopyToTemp(ctx context.Context, src string) (string, error) {
	return i.copyToTemp(ctx, src)
}

// SetRegistrySave replaces the installer's registry-save seam for testing,
// preventing writes to the real ~/.config/agentpack/packages/ directory.
func (i *Installer) SetRegistrySave(fn func(*registry.PackageManifest) error) {
	i.registrySave = fn
}

// SetRegistryLoad replaces the installer's registry-load seam for testing,
// preventing reads from the real ~/.config/agentpack/packages/ directory.
func (i *Installer) SetRegistryLoad(fn func(string) (*registry.PackageManifest, error)) {
	i.registryLoad = fn
}

// NameFromSource exposes nameFromSource for testing.
func NameFromSource(source string) string {
	return nameFromSource(source)
}

// HumanSize exposes cli.HumanSize for testing.
func HumanSize(bytes int64) string {
	return cli.HumanSize(bytes)
}

// CopyFileAtomic exposes copyFileAtomic for testing.
func CopyFileAtomic(src, dst string) error {
	return copyFileAtomic(src, dst)
}

// RegistrySource exposes registrySource for testing.
func RegistrySource(opts Options) string {
	return registrySource(opts)
}

// BuildContentMap exposes buildContentMap for testing.
func BuildContentMap(files []archive.FileEntry) (map[string][]byte, error) {
	return buildContentMap(files)
}

// WalkContentDir exposes walkContentDir for testing.
func WalkContentDir(cloneDir, root string) ([]archive.FileEntry, error) {
	return walkContentDir(cloneDir, root)
}

// AutoPackageWithVersion exposes autoPackageWithVersion for testing.
func AutoPackageWithVersion(
	ctx context.Context,
	cloneDir string,
	name string,
	sha string,
	version string,
	skillFilter []string,
	agentFilter []string,
) (string, error) {
	return autoPackageWithVersion(ctx, cloneDir, name, sha, version, skillFilter, agentFilter)
}

// BuildContentEntries exposes buildContentEntries for testing.
func BuildContentEntries(meta *metadata.Metadata, sourceDir string) []target.ContentEntry {
	return buildContentEntries(meta, sourceDir)
}

// VerifyArchiveSidecar exposes verifyArchiveSidecar for testing.
func VerifyArchiveSidecar(
	ctx context.Context,
	f fetcher.Fetcher,
	source, archivePath string,
) (bool, error) {
	return verifyArchiveSidecar(ctx, f, source, archivePath)
}

// HasMetadataYAML exposes hasMetadataYAML for testing.
func HasMetadataYAML(dir string) bool {
	return hasMetadataYAML(dir)
}

// HasContentDirs exposes hasContentDirs for testing.
func HasContentDirs(dir string) bool {
	return hasContentDirs(dir)
}

// MergeFiles exposes mergeFiles for testing.
func MergeFiles(existing, incoming []registry.InstalledFile) []registry.InstalledFile {
	return mergeFiles(existing, incoming)
}

// ParseSourceExported exposes ParseSource for testing from outside the package.
// (ParseSource is already exported, but this alias follows the export_test pattern.)
var ParseSourceExported = ParseSource

// NewWithTargets returns an Installer whose target resolver always yields the
// given targets, letting tests inject mocks without global registration. Each
// installer owns its resolver, so tests stay parallel-safe.
func NewWithTargets(targets []target.Target) *Installer {
	i := New()
	i.resolveTargets = func([]string) ([]target.Target, error) { return targets, nil }

	return i
}

// DefaultResolveTargets exposes defaultResolveTargets for testing.
func DefaultResolveTargets(names []string) ([]target.Target, error) {
	return defaultResolveTargets(names)
}
