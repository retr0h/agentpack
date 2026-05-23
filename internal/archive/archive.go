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

// Package archive handles creation and extraction of .claudia tarballs.
package archive

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FileEntry describes a single file to include in an archive.
// Either Src (path on disk) or Content (in-memory bytes) must be set.
type FileEntry struct {
	Src         string // absolute path to read from disk; empty for virtual files
	ArchivePath string // relative path inside the tarball
	Content     []byte // in-memory content; used when Src is empty
	Mode        int64  // file mode; 0 defaults to 0644
}

// Create writes a gzipped tarball at outputPath containing the given files.
func Create(outputPath string, files []FileEntry) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	dirs := make(map[string]bool)

	for _, fe := range files {
		if err := ensureDirs(tw, fe.ArchivePath, dirs); err != nil {
			return err
		}

		if fe.Src != "" {
			if err := addFileFromDisk(tw, fe); err != nil {
				return err
			}
		} else {
			if err := addVirtualFile(tw, fe); err != nil {
				return err
			}
		}
	}

	return nil
}

// Extract unpacks a gzipped tarball at archivePath into destDir.
// Symlinks and paths that escape destDir are rejected.
func Extract(archivePath string, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	cleanDest := filepath.Clean(destDir)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			return fmt.Errorf("symlinks not allowed: %s", hdr.Name)
		}

		target := filepath.Join(destDir, filepath.Clean(hdr.Name))
		if !strings.HasPrefix(target, cleanDest+string(filepath.Separator)) && target != cleanDest {
			return fmt.Errorf("path traversal detected: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return fmt.Errorf("mkdir %s: %w", hdr.Name, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("mkdir for %s: %w", hdr.Name, err)
			}
			if err := extractFile(tr, target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		}
	}

	return nil
}

func addFileFromDisk(tw *tar.Writer, fe FileEntry) error {
	info, err := os.Stat(fe.Src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", fe.Src, err)
	}

	mode := fe.Mode
	if mode == 0 {
		mode = int64(info.Mode())
	}

	hdr := &tar.Header{
		Name: fe.ArchivePath,
		Size: info.Size(),
		Mode: mode,
	}

	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write header %s: %w", fe.ArchivePath, err)
	}

	f, err := os.Open(fe.Src)
	if err != nil {
		return fmt.Errorf("open %s: %w", fe.Src, err)
	}
	defer f.Close()

	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("copy %s: %w", fe.ArchivePath, err)
	}

	return nil
}

func addVirtualFile(tw *tar.Writer, fe FileEntry) error {
	mode := fe.Mode
	if mode == 0 {
		mode = 0o644
	}

	hdr := &tar.Header{
		Name: fe.ArchivePath,
		Size: int64(len(fe.Content)),
		Mode: mode,
	}

	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write header %s: %w", fe.ArchivePath, err)
	}

	if _, err := tw.Write(fe.Content); err != nil {
		return fmt.Errorf("write %s: %w", fe.ArchivePath, err)
	}

	return nil
}

// ensureDirs adds directory entries for every parent of archivePath that
// hasn't been added yet.
func ensureDirs(tw *tar.Writer, archivePath string, seen map[string]bool) error {
	dir := filepath.Dir(archivePath)
	if dir == "." {
		return nil
	}

	parts := strings.Split(filepath.ToSlash(dir), "/")
	for i := range parts {
		d := strings.Join(parts[:i+1], "/") + "/"
		if seen[d] {
			continue
		}
		seen[d] = true

		hdr := &tar.Header{
			Name:     d,
			Typeflag: tar.TypeDir,
			Mode:     0o755,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("write dir header %s: %w", d, err)
		}
	}

	return nil
}

func extractFile(r io.Reader, target string, mode os.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}

	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create %s: %w", target, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("extract %s: %w", target, err)
	}

	return nil
}
