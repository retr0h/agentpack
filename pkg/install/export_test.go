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
	"github.com/retr0h/agentpack/internal/gitutil"
	"github.com/retr0h/agentpack/internal/metadata"
	"github.com/retr0h/agentpack/pkg/registry"
	"github.com/retr0h/agentpack/internal/target"
)

// StoreArchive exposes storeArchive for testing.
func StoreArchive(ctx context.Context, srcPath, name, sha string) (string, error) {
	return storeArchive(ctx, srcPath, name, sha)
}

// FilteredSubdirs exposes filteredSubdirs for testing.
func FilteredSubdirs(contentDir string, filter []string) []string {
	return filteredSubdirs(contentDir, filter)
}

// ArchivesDir exposes archivesDir for testing.
func ArchivesDir() (string, error) {
	return archivesDir()
}

// SetArchivesDir replaces archivesDirFunc for testing and returns a restore fn.
func SetArchivesDir(fn func() (string, error)) func() {
	orig := archivesDirFunc
	archivesDirFunc = fn
	return func() { archivesDirFunc = orig }
}

// SetArchivesDirHome replaces archivesDirHome for testing and returns a restore fn.
func SetArchivesDirHome(fn func() (string, error)) func() {
	orig := archivesDirHome
	archivesDirHome = fn
	return func() { archivesDirHome = orig }
}

// ShortSHA exposes gitutil.ShortSHA for testing.
func ShortSHA(sha string) string {
	return gitutil.ShortSHA(sha)
}

// CopyDir exposes copyDir for testing.
func CopyDir(ctx context.Context, src string, dst string) error {
	return copyDir(ctx, src, dst)
}

// CopyFile exposes copyFile for testing.
func CopyFile(src string, dst string) error {
	return copyFile(src, dst)
}

// FindChecksums exposes findChecksums for testing.
func FindChecksums(ctx context.Context, dir string) (string, error) {
	return findChecksums(ctx, dir)
}

// FindAndReadMetadata exposes findAndReadMetadata for testing.
func FindAndReadMetadata(ctx context.Context, dir string) (*metadata.Metadata, error) {
	return findAndReadMetadata(ctx, dir)
}

// SetOsCreateTemp replaces osCreateTemp for testing.
func SetOsCreateTemp(fn func(string, string) (*os.File, error)) func() {
	orig := osCreateTemp
	osCreateTemp = fn

	return func() { osCreateTemp = orig }
}

// SetOsMkdirTemp replaces osMkdirTemp for testing.
func SetOsMkdirTemp(fn func(string, string) (string, error)) func() {
	orig := osMkdirTemp
	osMkdirTemp = fn

	return func() { osMkdirTemp = orig }
}

// CreateTempAlwaysFails is an osCreateTemp that always returns an error.
func CreateTempAlwaysFails(_, _ string) (*os.File, error) {
	return nil, errors.New("simulated create temp failure")
}

// MkdirTempAlwaysFails is an osMkdirTemp that always returns an error.
func MkdirTempAlwaysFails(_, _ string) (string, error) {
	return "", errors.New("simulated mkdir temp failure")
}

// MkdirTempFailAfterN returns a replacement for osMkdirTemp that succeeds
// on the first n calls and then returns an error.
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

// CopyToTemp exposes copyToTemp for testing.
func CopyToTemp(ctx context.Context, src string) (string, error) {
	return copyToTemp(ctx, src)
}

// SetRegistrySave replaces the registrySave function for testing. It returns
// a restore function that callers should defer so each test cleans up after
// itself. Use this in non-parallel tests to prevent registry writes from
// polluting the real ~/.config/agentpack/packages/ directory.
func SetRegistrySave(fn func(*registry.PackageManifest) error) func() {
	orig := registrySave
	registrySave = fn

	return func() { registrySave = orig }
}

// SetRegistryLoad replaces the registryLoad function for testing. It returns
// a restore function that callers should defer so each test cleans up after
// itself. Use this in non-parallel tests to prevent reads from the real
// ~/.config/agentpack/packages/ directory.
func SetRegistryLoad(fn func(string) (*registry.PackageManifest, error)) func() {
	orig := registryLoad
	registryLoad = fn

	return func() { registryLoad = orig }
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

// ComputeChecksums exposes computeChecksums for testing.
func ComputeChecksums(files []archive.FileEntry) ([]byte, error) {
	return computeChecksums(files)
}

// BuildContentEntries exposes buildContentEntries for testing.
func BuildContentEntries(meta *metadata.Metadata, sourceDir string) []target.ContentEntry {
	return buildContentEntries(meta, sourceDir)
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
