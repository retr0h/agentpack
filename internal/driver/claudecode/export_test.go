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

package claudecode

import (
	"context"
	"os"
)

// SetUserHome replaces userHomeFunc for testing.
func SetUserHome(cc *ClaudeCode, fn func() (string, error)) {
	cc.userHomeFunc = fn
}

// SetOsMkdirAll replaces mkdirAllFunc for testing.
func SetOsMkdirAll(cc *ClaudeCode, fn func(string, os.FileMode) error) {
	cc.mkdirAllFunc = fn
}

// CopyFile exposes copyFile for testing.
func CopyFile(src, dst string) error {
	return copyFile(src, dst)
}

// CopyTree exposes copyTree for testing.
func CopyTree(mkdirAll func(string, os.FileMode) error, src, dst string) error {
	return copyTree(mkdirAll, src, dst)
}

// ShortSHA exposes shortSHA for testing.
func ShortSHA(sha string) string {
	return shortSHA(sha)
}

// FormatDate exposes formatDate for testing.
func FormatDate(ts string) string {
	return formatDate(ts)
}

// InstallMCP exposes installMCP for testing.
func InstallMCP(ctx context.Context, cc *ClaudeCode, srcDir, settingsPath string) error {
	return cc.installMCP(ctx, srcDir, settingsPath)
}

// InstallHooks exposes installHooks for testing.
func InstallHooks(
	ctx context.Context,
	cc *ClaudeCode,
	srcDir, settingsPath, pluginName string,
) error {
	return cc.installHooks(ctx, srcDir, settingsPath, pluginName)
}

// InstallSettings exposes installSettings for testing.
func InstallSettings(ctx context.Context, cc *ClaudeCode, srcDir, settingsPath string) error {
	return cc.installSettings(ctx, srcDir, settingsPath)
}
