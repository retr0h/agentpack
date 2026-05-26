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

// Package inspect opens a .agentpack archive and reports its contents.
//
// Usage:
//
//	ins := inspect.New()
//	result, err := ins.Run(ctx, inspect.Options{Path: "plugin-v1.0.0.agentpack"})
//	if err != nil { ... }
//	for _, f := range result.Files {
//	    fmt.Printf("%s %s\n", f.Path, f.SHA256)
//	}
package inspect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/retr0h/agentpack/internal/archive"
	"github.com/retr0h/agentpack/internal/checksum"
	"github.com/retr0h/agentpack/internal/safety"
)

// osMkdirTemp is swappable for testing.
var osMkdirTemp = os.MkdirTemp

// Inspector opens a .agentpack archive and reports its contents.
type Inspector struct{}

// New returns a new Inspector ready to inspect archives.
func New() *Inspector { return &Inspector{} }

// Options configures an inspect run.
type Options struct {
	Path string
}

// FileEntry describes a single file found in the archive.
type FileEntry struct {
	Path     string
	Size     int64
	SHA256   string
	Verified bool
}

// Result holds the outcome of inspecting a .agentpack archive.
type Result struct {
	Name    string
	Version string
	Built   string
	SHA     string
	Files   []FileEntry
	Total   int64
	// Content holds the safety classification embedded in the archive metadata.
	// Nil when the archive predates ADR-005.
	Content *safety.Classification
}

// archiveMetadata mirrors the fields written into .agentpack/metadata.json.
type archiveMetadata struct {
	Name           string                 `json:"name"`
	Version        string                 `json:"version"`
	GitCommitSHA   string                 `json:"gitCommitSHA"`
	BuildTimestamp string                 `json:"buildTimestamp"`
	Content        *safety.Classification `json:"content,omitempty"`
}

// Run extracts archivePath to a temp dir, reads metadata and checksums, walks
// all files, and returns a populated Result.
func (ins *Inspector) Run(ctx context.Context, opts Options) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tmpDir, err := osMkdirTemp("", "agentpack-inspect-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := archive.Extract(ctx, opts.Path, tmpDir); err != nil {
		return nil, fmt.Errorf("extract: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	metaPath := filepath.Join(tmpDir, ".agentpack", "metadata.json")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("read metadata.json: %w", err)
	}

	var meta archiveMetadata
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return nil, fmt.Errorf("parse metadata.json: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	checksumPath := filepath.Join(tmpDir, ".agentpack", "checksums.txt")
	entries, err := checksum.ReadFile(checksumPath)
	if err != nil {
		return nil, fmt.Errorf("read checksums.txt: %w", err)
	}

	// Build a lookup map from the checksums file for O(1) verification.
	expected := make(map[string]string, len(entries))
	for _, e := range entries {
		expected[e.Path] = e.Hash
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Walk all extracted files, skipping the .agentpack/ directory itself
	// (its children are appended explicitly after the walk).
	agentpackDir := filepath.Join(tmpDir, ".agentpack") + string(filepath.Separator)
	var files []FileEntry

	err = filepath.WalkDir(tmpDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasPrefix(path, agentpackDir) {
			return nil
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		rel, err := filepath.Rel(tmpDir, path)
		if err != nil {
			return fmt.Errorf("rel path %s: %w", path, err)
		}
		rel = filepath.ToSlash(rel)

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}

		hash, err := computeFileHash(path)
		if err != nil {
			return err
		}

		files = append(files, FileEntry{
			Path:     rel,
			Size:     info.Size(),
			SHA256:   hash,
			Verified: expected[rel] == hash,
		})

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk archive: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Append the two .agentpack meta-files so they appear in the listing.
	for _, name := range []string{"metadata.json", "checksums.txt"} {
		absPath := filepath.Join(tmpDir, ".agentpack", name)
		rel := ".agentpack/" + name

		info, err := os.Stat(absPath)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", rel, err)
		}

		var data []byte
		if name == "metadata.json" {
			data = metaData
		} else {
			data, err = os.ReadFile(absPath)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", rel, err)
			}
		}

		hash := computeBytesHash(data)
		files = append(files, FileEntry{
			Path:     rel,
			Size:     info.Size(),
			SHA256:   hash,
			Verified: expected[rel] == hash,
		})
	}

	var total int64
	for _, f := range files {
		total += f.Size
	}

	return &Result{
		Name:    meta.Name,
		Version: meta.Version,
		Built:   meta.BuildTimestamp,
		SHA:     meta.GitCommitSHA,
		Files:   files,
		Total:   total,
		Content: meta.Content,
	}, nil
}

func computeFileHash(path string) (string, error) {
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

func computeBytesHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
