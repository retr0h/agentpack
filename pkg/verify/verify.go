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
//
// Usage:
//
//	v := verify.New()
//	result, err := v.Run(ctx, verify.Options{ArchivePath: "/path/to/plugin-v1.0.0.agentpack"})
//	if err != nil { ... }
//	for _, f := range result.Files {
//	    if !f.OK { fmt.Printf("FAIL %s: %s\n", f.Path, f.Err) }
//	}
//
// Two archive formats are supported (ADR-009):
//
//   - Old format: archive contains .agentpack/checksums.txt. Verify extracts the
//     archive to a temp directory, reads checksums.txt, and recomputes the SHA256
//     of every listed file. Per-file failures are surfaced through Result.Files.
//
//   - New format (ADR-009): archive has no checksums.txt. Archive-level integrity
//     is provided by a .sha256 sidecar file written by `agentpack build`. When no
//     checksums.txt is found, Run returns a Result with an empty Files slice
//     (no error). Sidecar verification is handled at the command layer before
//     calling Run.
//
// A non-nil error is returned only for I/O failures that prevent verification
// from running.
package verify

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/retr0h/agentpack/internal/archive"
	"github.com/retr0h/agentpack/internal/checksum"
)

// errChecksumsNotFound is returned by findChecksums when the archive contains
// no checksums.txt (new-format archive per ADR-009). It is intentionally
// distinct from I/O errors so Run can skip internal verification gracefully.
var errChecksumsNotFound = errors.New("checksums.txt not found in archive")

// Options configures a verify run.
type Options struct {
	ArchivePath string
}

// osMkdirTemp is swappable for testing.
var osMkdirTemp = os.MkdirTemp

// FileResult holds the verification outcome for a single file in the archive.
type FileResult struct {
	Path string `json:"path"`
	OK   bool   `json:"ok"`
	Err  string `json:"err,omitempty"`
}

// Result holds the outcome of verifying a .agentpack archive.
type Result struct {
	ArchiveName string       `json:"archiveName"`
	Files       []FileResult `json:"files"`
}

// Verifier orchestrates an archive verification run.
type Verifier struct{}

// New returns a new Verifier.
func New() *Verifier { return &Verifier{} }

// Run extracts a .agentpack archive to a temp directory, locates checksums.txt,
// and verifies every file listed in it. It returns a Result describing each
// file's verification status. A non-nil error is returned only when the
// overall operation cannot proceed (e.g. cannot extract or find checksums.txt).
func (v *Verifier) Run(ctx context.Context, opts Options) (*Result, error) {
	archivePath := opts.ArchivePath

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
		// New-format archives (ADR-009) omit checksums.txt. Archive-level
		// integrity is verified via the .sha256 sidecar before Run is called.
		// Treat the missing file as a successful no-op rather than an error.
		if errors.Is(err, errChecksumsNotFound) {
			return &Result{
				ArchiveName: filepath.Base(archivePath),
				Files:       []FileResult{},
			}, nil
		}

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
		return "", errChecksumsNotFound
	}

	return found, nil
}
