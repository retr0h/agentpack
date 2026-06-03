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

package sync

import (
	"context"

	"github.com/retr0h/agentpack/pkg/build"
	"github.com/retr0h/agentpack/pkg/install"
)

// Fetcher retrieves a git source into a local directory. Implement this to
// inject a test double in place of the default git backend; a nil Fetcher in
// Options falls back to the production implementation.
type Fetcher interface {
	Fetch(ctx context.Context, source, dest string) error
}

// Builder builds agentpack archives from a directory.
type Builder interface {
	Build(ctx context.Context, dir string) ([]build.Result, error)
}

// Installer installs a single agentpack archive.
type Installer interface {
	Install(ctx context.Context, source string) (*install.Result, error)
}
