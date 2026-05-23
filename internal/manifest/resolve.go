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

package manifest

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/avfs/avfs"
)

// FilePair holds an absolute source path and its relative destination path
// within the archive.
type FilePair struct {
	Src  string // absolute path to source file
	Dest string // relative destination path in the archive
}

// ResolveEntries expands a slice of Entry values into concrete FilePair
// mappings. Each entry is resolved relative to baseDir using vfs.
//
// Rules:
//   - Glob entry: expand the glob relative to baseDir; the relative path from
//     baseDir becomes the destination. Matching zero files is an error.
//   - Src/dest with a directory dest (trailing slash or bare directory name):
//     expand the src glob relative to baseDir; use dest + filename as the
//     destination.
//   - Src/dest with a file dest: map exactly one source file to the renamed
//     destination.
//   - An entry with neither Glob nor Src is an error.
//   - Empty or nil entries returns an empty slice with no error.
func ResolveEntries(ctx context.Context, vfs avfs.VFS, baseDir string, entries []Entry) ([]FilePair, error) {
	if len(entries) == 0 {
		return []FilePair{}, nil
	}

	var pairs []FilePair

	for _, e := range entries {
		switch {
		case e.Glob != "":
			got, err := resolveGlob(vfs, baseDir, e.Glob)
			if err != nil {
				return nil, err
			}
			pairs = append(pairs, got...)

		case e.Src != "":
			got, err := resolveSrcDest(vfs, baseDir, e.Src, e.Dest)
			if err != nil {
				return nil, err
			}
			pairs = append(pairs, got...)

		default:
			return nil, fmt.Errorf("entry has neither glob nor src")
		}
	}

	return pairs, nil
}

// resolveGlob expands a glob pattern relative to baseDir using vfs and returns
// one FilePair per matched file, using the relative path from baseDir as the
// destination.
func resolveGlob(vfs avfs.VFS, baseDir, pattern string) ([]FilePair, error) {
	abs := filepath.Join(baseDir, pattern)
	matches, err := vfs.Glob(abs)
	if err != nil {
		return nil, fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("pattern '%s' matched no files", pattern)
	}

	pairs := make([]FilePair, 0, len(matches))
	for _, m := range matches {
		rel, err := vfs.Rel(baseDir, m)
		if err != nil {
			return nil, fmt.Errorf("computing relative path for %q: %w", m, err)
		}
		pairs = append(pairs, FilePair{Src: m, Dest: rel})
	}
	return pairs, nil
}

// resolveSrcDest handles an Entry with an explicit Src (possibly a glob) and
// an optional Dest.
//
//   - If dest ends with "/" or is an existing directory, it is treated as a
//     directory prefix; the source filename is appended.
//   - Otherwise dest is used as-is (rename).
//   - If src contains no glob metacharacters and the resolved file does not
//     exist, an error is returned.
func resolveSrcDest(vfs avfs.VFS, baseDir, src, dest string) ([]FilePair, error) {
	absSrc := filepath.Join(baseDir, src)

	// Determine whether src is a glob.
	isGlob := strings.ContainsAny(src, "*?[")

	if isGlob {
		matches, err := vfs.Glob(absSrc)
		if err != nil {
			return nil, fmt.Errorf("invalid glob pattern %q: %w", src, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("pattern '%s' matched no files", src)
		}

		pairs := make([]FilePair, 0, len(matches))
		for _, m := range matches {
			d := destPath(dest, filepath.Base(m))
			pairs = append(pairs, FilePair{Src: m, Dest: d})
		}
		return pairs, nil
	}

	// Non-glob: the file must exist.
	if _, err := vfs.Stat(absSrc); err != nil {
		if avfs.IsNotExist(err) {
			return nil, fmt.Errorf("src file not found: %s", src)
		}
		return nil, fmt.Errorf("stat %q: %w", src, err)
	}

	d := destPath(dest, filepath.Base(absSrc))
	return []FilePair{{Src: absSrc, Dest: d}}, nil
}

// destPath combines a destination prefix with a filename. When prefix ends
// with "/" or is empty, the result is prefix+filename. Otherwise prefix is
// returned as-is (explicit rename).
func destPath(prefix, filename string) string {
	if prefix == "" {
		return filename
	}
	if strings.HasSuffix(prefix, "/") {
		return prefix + filename
	}
	return prefix
}
