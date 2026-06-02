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

package devin

import (
	"context"
	"os"

	"github.com/retr0h/agentpack/pkg/target"
)

// SetConfigDir replaces configDirFunc for testing.
func SetConfigDir(d *Devin, fn func() (string, error)) {
	d.configDirFunc = fn
}

// SetCwd replaces cwdFunc for testing.
func SetCwd(d *Devin, fn func() (string, error)) {
	d.cwdFunc = fn
}

// SetOsMkdirAll replaces mkdirAllFunc for testing.
func SetOsMkdirAll(d *Devin, fn func(string, os.FileMode) error) {
	d.mkdirAllFunc = fn
}

// InstallMCP exposes installMCP for testing.
func InstallMCP(ctx context.Context, d *Devin, srcDir, mcpPath string) error {
	return d.installMCP(ctx, srcDir, mcpPath)
}

// InstallHooks exposes installHooks for testing.
func InstallHooks(
	ctx context.Context,
	d *Devin,
	srcDir, hooksPath, pluginName string,
) error {
	return d.installHooks(ctx, srcDir, hooksPath, pluginName)
}

// MCPConfigPath exposes mcpConfigPath for testing.
func MCPConfigPath(d *Devin, opts target.InstallOpts) (string, error) {
	return d.mcpConfigPath(opts)
}

// HooksPath exposes hooksPath for testing.
func HooksPath(d *Devin, opts target.InstallOpts) (string, error) {
	return d.hooksPath(opts)
}
