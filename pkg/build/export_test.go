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

// Package build export_test.go exposes private helpers for white-box testing.
package build

import (
	"context"

	"github.com/avfs/avfs"

	"github.com/retr0h/claudia/pkg/archive"
	"github.com/retr0h/claudia/pkg/checksum"
)

// FileEntry aliases archive.FileEntry for use in build_test.go without
// importing archive directly.
type FileEntry = archive.FileEntry

// ChecksumEntry aliases checksum.Entry for use in build_test.go.
type ChecksumEntry = checksum.Entry

// ComputeArchiveChecksums exposes computeArchiveChecksums for testing.
func ComputeArchiveChecksums(
	ctx context.Context,
	vfs avfs.VFS,
	files []archive.FileEntry,
) ([]checksum.Entry, error) {
	return computeArchiveChecksums(ctx, vfs, files)
}
