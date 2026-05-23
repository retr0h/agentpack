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

// Package checksum handles per-file SHA256 checksumming and verification.
package checksum

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/avfs/avfs"
)

// Entry holds a file path and its expected SHA256 hash.
type Entry struct {
	Hash string
	Path string
}

// Result holds the verification outcome for a single file.
type Result struct {
	Path string
	OK   bool
	Err  string
}

// ComputeBytes returns the SHA256 hash of data as a 64-char hex string.
func ComputeBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// ComputeFile reads the file at path using vfs and returns its SHA256 hash as
// a 64-char hex string.
func ComputeFile(_ context.Context, vfs avfs.VFS, path string) (string, error) {
	f, err := vfs.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// WriteFile writes entries to path in sha256sum format:
//
//	{hash}  {path}\n
//
// (two spaces between hash and path, matching the sha256sum(1) convention).
func WriteFile(_ context.Context, vfs avfs.VFS, path string, entries []Entry) error {
	f, err := vfs.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}

	w := bufio.NewWriter(f)
	for _, e := range entries {
		if _, err := fmt.Fprintf(w, "%s  %s\n", e.Hash, e.Path); err != nil {
			_ = f.Close()
			return fmt.Errorf("write entry: %w", err)
		}
	}

	if err := w.Flush(); err != nil {
		_ = f.Close()
		return fmt.Errorf("flush %s: %w", path, err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}

	return nil
}

// ReadFile parses a sha256sum-format file at path and returns its entries.
func ReadFile(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Format: "{hash}  {path}" — two spaces as separator.
		hash, filePath, found := strings.Cut(line, "  ")
		if !found {
			return nil, fmt.Errorf("line %d: invalid format", lineNum)
		}

		entries = append(entries, Entry{Hash: hash, Path: filePath})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}

	return entries, nil
}

// Verify hashes each file described by entries (resolved relative to baseDir)
// and compares against the expected hash. It returns one Result per entry.
// A non-nil error is returned only for unexpected I/O failures unrelated to
// individual file access; per-file failures are surfaced via Result.OK/Err.
func Verify(ctx context.Context, baseDir string, entries []Entry) ([]Result, error) {
	results := make([]Result, 0, len(entries))

	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		fullPath := filepath.Join(baseDir, e.Path)

		got, err := computeFileOS(fullPath)
		if err != nil {
			results = append(results, Result{
				Path: e.Path,
				OK:   false,
				Err:  err.Error(),
			})

			continue
		}

		if got != e.Hash {
			results = append(results, Result{
				Path: e.Path,
				OK:   false,
				Err:  fmt.Sprintf("checksum mismatch: got %s, want %s", got, e.Hash),
			})

			continue
		}

		results = append(results, Result{Path: e.Path, OK: true})
	}

	return results, nil
}

// computeFileOS reads the file at path using the real OS and returns its
// SHA256 hash. Used by Verify which always operates on extracted archives
// in real temp dirs.
func computeFileOS(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
