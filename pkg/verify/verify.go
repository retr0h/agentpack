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

// Package verify orchestrates the agentpack archive verification pipeline.
package verify

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/retr0h/agentpack/pkg/archive"
	"github.com/retr0h/agentpack/pkg/checksum"
)

// osMkdirTemp is swappable for testing.
var osMkdirTemp = os.MkdirTemp

// FileResult holds the verification outcome for a single file in the archive.
type FileResult struct {
	Path string
	OK   bool
	Err  string
}

// Result holds the outcome of verifying a .agentpack archive.
type Result struct {
	ArchiveName string
	Files       []FileResult
}

// Run extracts a .agentpack archive to a temp directory, locates checksums.txt,
// and verifies every file listed in it. It returns a Result describing each
// file's verification status. A non-nil error is returned only when the
// overall operation cannot proceed (e.g. cannot extract or find checksums.txt).
func Run(ctx context.Context, archivePath string) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tmpDir, err := osMkdirTemp("", "agentpack-verify-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := archive.Extract(ctx, archivePath, tmpDir); err != nil {
		return nil, fmt.Errorf("extract: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	checksumFile, err := findChecksums(tmpDir)
	if err != nil {
		return nil, err
	}

	entries, err := checksum.ReadFile(checksumFile)
	if err != nil {
		return nil, fmt.Errorf("reading checksums: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	rawResults, err := checksum.Verify(ctx, tmpDir, entries)
	if err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}

	fileResults := make([]FileResult, len(rawResults))
	for i, r := range rawResults {
		fileResults[i] = FileResult{
			Path: r.Path,
			OK:   r.OK,
			Err:  r.Err,
		}
	}

	return &Result{
		ArchiveName: filepath.Base(archivePath),
		Files:       fileResults,
	}, nil
}

// findChecksums walks dir searching for a checksums.txt file inside a
// .agentpack directory. Returns an error if not found.
func findChecksums(dir string) (string, error) {
	var found string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "checksums.txt" && strings.Contains(path, ".agentpack") {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("searching for checksums.txt: %w", err)
	}

	if found == "" {
		return "", fmt.Errorf("checksums.txt not found in archive")
	}

	return found, nil
}
