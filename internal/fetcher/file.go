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

package fetcher

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Swappable for testing.
var (
	osUserHomeDir = os.UserHomeDir
	ioCopyFile    = io.Copy
	osCreateFile  = func(name string) (io.WriteCloser, error) {
		return os.Create(name)
	}
)

// FileFetcher copies a local file to a destination path.
type FileFetcher struct{}

// Fetch copies the file at source to dest. It expands a leading ~ to the
// user's home directory. The context is checked before the copy begins.
func (f *FileFetcher) Fetch(ctx context.Context, source string, dest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	src, err := expandHome(source)
	if err != nil {
		return fmt.Errorf("expand path: %w", err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = in.Close() }()

	if err := ctx.Err(); err != nil {
		return err
	}

	out, err := osCreateFile(dest)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer func() { _ = out.Close() }()

	if _, err := ioCopyFile(out, in); err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	if err = out.Close(); err != nil {
		return fmt.Errorf("close dest: %w", err)
	}

	return nil
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~/") {
		return path, nil
	}

	home, err := osUserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}

	return filepath.Join(home, path[2:]), nil
}
