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

// Package safety classifies archive files into safe, executable, and binary
// categories per ADR-005.
package safety

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Category describes the safety classification of a file.
type Category string

const (
	// Safe files are plain text with known-safe extensions.
	Safe Category = "safe"
	// Executable files are text-based scripts that require user consent.
	Executable Category = "executable"
	// Binary files are compiled executables rejected at build time.
	Binary Category = "binary"
)

// Classification holds the results of classifying an archive's file set.
type Classification struct {
	Safe       []string `json:"safe"`
	Executable []string `json:"executable"`
}

var safeExts = map[string]struct{}{
	".md":   {},
	".json": {},
	".yaml": {},
	".yml":  {},
	".txt":  {},
	".toml": {},
}

var executableExts = map[string]struct{}{
	".sh":  {},
	".py":  {},
	".js":  {},
	".ts":  {},
	".rb":  {},
	".pl":  {},
	".lua": {},
}

// Classify takes a map of archive-relative file paths to their content and
// returns a Classification. Returns an error if any binary file is detected;
// binary files are never allowed in archives.
func Classify(files map[string][]byte) (*Classification, error) {
	c := &Classification{}

	for path, content := range files {
		cat := ClassifyFile(path, content)

		switch cat {
		case Binary:
			return nil, fmt.Errorf(
				"binary file detected: %s (%s)",
				path,
				binaryDescription(content),
			)
		case Executable:
			c.Executable = append(c.Executable, path)
		default:
			c.Safe = append(c.Safe, path)
		}
	}

	return c, nil
}

// ClassifyFile returns the Category for a single file based on its extension
// and content. Binary detection via magic bytes takes priority over extension
// and shebang checks.
func ClassifyFile(path string, content []byte) Category {
	if isBinary(content) {
		return Binary
	}

	if hasShebang(content) {
		return Executable
	}

	ext := strings.ToLower(filepath.Ext(path))

	if _, ok := executableExts[ext]; ok {
		return Executable
	}

	if _, ok := safeExts[ext]; ok {
		return Safe
	}

	return Safe
}

func hasShebang(content []byte) bool {
	return len(content) >= 2 && content[0] == '#' && content[1] == '!'
}

func isBinary(content []byte) bool {
	if len(content) < 2 {
		return false
	}

	// PE: MZ
	if content[0] == 'M' && content[1] == 'Z' {
		return true
	}

	if len(content) < 4 {
		return false
	}

	// ELF
	if content[0] == 0x7f && content[1] == 'E' && content[2] == 'L' && content[3] == 'F' {
		return true
	}

	// Mach-O: \xfe\xed\xfa\xce or \xfe\xed\xfa\xcf
	if content[0] == 0xfe && content[1] == 0xed && content[2] == 0xfa &&
		(content[3] == 0xce || content[3] == 0xcf) {
		return true
	}

	// Mach-O fat binary: \xca\xfe\xba\xbe
	if content[0] == 0xca && content[1] == 0xfe && content[2] == 0xba && content[3] == 0xbe {
		return true
	}

	return false
}

func binaryDescription(content []byte) string {
	if len(content) >= 2 && content[0] == 'M' && content[1] == 'Z' {
		return "PE executable"
	}

	if len(content) >= 4 {
		if content[0] == 0x7f && content[1] == 'E' && content[2] == 'L' && content[3] == 'F' {
			return "ELF executable"
		}

		if content[0] == 0xfe && content[1] == 0xed && content[2] == 0xfa &&
			(content[3] == 0xce || content[3] == 0xcf) {
			return "Mach-O executable"
		}

		if content[0] == 0xca && content[1] == 0xfe && content[2] == 0xba && content[3] == 0xbe {
			return "Mach-O fat binary"
		}
	}

	return "binary"
}
