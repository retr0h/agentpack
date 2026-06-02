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

package junie

import (
	"context"
	"os"

	"github.com/retr0h/agentpack/pkg/target"
)

// SetUserHome replaces userHomeFunc for testing.
func SetUserHome(j *Junie, fn func() (string, error)) {
	j.userHomeFunc = fn
}

// SetCwd replaces cwdFunc for testing.
func SetCwd(j *Junie, fn func() (string, error)) {
	j.cwdFunc = fn
}

// SetOsMkdirAll replaces mkdirAllFunc for testing.
func SetOsMkdirAll(j *Junie, fn func(string, os.FileMode) error) {
	j.mkdirAllFunc = fn
}

// InstallMCP exposes installMCP for testing.
func InstallMCP(ctx context.Context, j *Junie, srcDir, mcpPath string) error {
	return j.installMCP(ctx, srcDir, mcpPath)
}

// CopyTreeIfExists exposes copyTreeIfExists for testing.
func CopyTreeIfExists(ctx context.Context, src, dst string) error {
	return copyTreeIfExists(ctx, src, dst)
}

// CopyFile exposes copyFile for testing.
func CopyFile(src, dst string) error {
	return copyFile(src, dst)
}

// EnumerateFiles exposes enumerateFiles for testing.
func EnumerateFiles(ctx context.Context, destDir, baseDir string) ([]target.InstalledFile, error) {
	return enumerateFiles(ctx, destDir, baseDir)
}
