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

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/retr0h/claudia/internal/archive"
	"github.com/retr0h/claudia/internal/checksum"
)

var verifyCmd = &cobra.Command{
	Use:   "verify <archive.claudia>",
	Short: "Verify checksums of a .claudia archive",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return runVerify(args[0])
	},
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}

func runVerify(archivePath string) error {
	fmt.Printf("claudia: verifying %s\n\n", filepath.Base(archivePath))

	tmpDir, err := os.MkdirTemp("", "claudia-verify-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := archive.Extract(archivePath, tmpDir); err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	checksumFile, err := findChecksums(tmpDir)
	if err != nil {
		return err
	}

	entries, err := checksum.ReadFile(checksumFile)
	if err != nil {
		return fmt.Errorf("reading checksums: %w", err)
	}

	results, err := checksum.Verify(tmpDir, entries)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}

	passed := 0
	failed := 0

	for _, r := range results {
		if r.OK {
			fmt.Printf("  %-60s OK\n", r.Path)
			passed++
		} else {
			fmt.Printf("  %-60s FAIL  %s\n", r.Path, r.Err)
			failed++
		}
	}

	total := passed + failed
	fmt.Printf("\n  %d/%d files verified\n", passed, total)

	if failed > 0 {
		return fmt.Errorf("%d file(s) failed verification", failed)
	}

	return nil
}

func findChecksums(dir string) (string, error) {
	var found string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "checksums.txt" && strings.Contains(path, ".claudia") {
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
