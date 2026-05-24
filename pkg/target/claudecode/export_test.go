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

// Package claudecode export_test.go exposes private helpers for white-box
// testing.
package claudecode

import (
	"context"
	"os"

	"github.com/retr0h/agentpack/pkg/manifest"
	"github.com/retr0h/agentpack/pkg/target"
)

// SetUserHome replaces the userHomeFunc on cc for the duration of a test.
func SetUserHome(cc *ClaudeCode, fn func() (string, error)) {
	cc.userHomeFunc = fn
}

// SetRenameFunc replaces the renameFunc on cc for the duration of a test.
func SetRenameFunc(cc *ClaudeCode, fn func(string, string) error) {
	cc.renameFunc = fn
}

// SetOsMkdirAll replaces the mkdirAllFunc on cc for the duration of a test.
func SetOsMkdirAll(cc *ClaudeCode, fn func(string, os.FileMode) error) {
	cc.mkdirAllFunc = fn
}

// SetOsRemoveAll replaces the removeAllFunc on cc for the duration of a test.
func SetOsRemoveAll(cc *ClaudeCode, fn func(string) error) {
	cc.removeAllFunc = fn
}

// WriteDescriptors exposes writeDescriptors for testing.
func WriteDescriptors(cc *ClaudeCode, destDir string, opts target.InstallOpts) error {
	return cc.writeDescriptors(destDir, opts)
}

// ReadManifestPlugin exposes readManifestPlugin for testing.
func ReadManifestPlugin(dir string, opts target.InstallOpts) (manifest.Plugin, error) {
	return readManifestPlugin(dir, opts)
}

// SynthPlugin exposes synthPlugin for testing.
func SynthPlugin(opts target.InstallOpts) manifest.Plugin {
	return synthPlugin(opts)
}

// CollectCommandPaths exposes collectCommandPaths for testing.
func CollectCommandPaths(dir string) []string {
	return collectCommandPaths(dir)
}

// CopyDir exposes copyDir for testing.
func CopyDir(ctx context.Context, src string, dst string) error {
	return copyDir(ctx, src, dst)
}

// CopyFile exposes copyFile for testing.
func CopyFile(src string, dst string) error {
	return copyFile(src, dst)
}

// ShortSHA exposes shortSHA for testing.
func ShortSHA(sha string) string {
	return shortSHA(sha)
}

// FormatDate exposes formatDate for testing.
func FormatDate(ts string) string {
	return formatDate(ts)
}
