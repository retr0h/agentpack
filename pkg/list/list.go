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

// Package list scans an installed plugin directory and returns metadata for
// every claudia-managed plugin found.
package list

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/retr0h/claudia/pkg/metadata"
)

// Entry represents a single installed claudia plugin.
type Entry struct {
	Name      string
	Version   string
	SHA       string
	Installed string // build timestamp from metadata.json
	Dir       string // path to the marketplace directory
}

// Run scans pluginDir/marketplaces/ for installed claudia plugins and returns
// them sorted by name. A directory is considered a claudia plugin when it
// contains a .claudia/metadata.json file.
func Run(pluginDir string) ([]Entry, error) {
	marketplacesDir := filepath.Join(pluginDir, "marketplaces")

	dirEntries, err := os.ReadDir(marketplacesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, fmt.Errorf("read marketplaces dir: %w", err)
	}

	var entries []Entry

	for _, de := range dirEntries {
		if !de.IsDir() {
			continue
		}

		dir := filepath.Join(marketplacesDir, de.Name())
		metaPath := filepath.Join(dir, ".claudia", "metadata.json")

		data, err := os.ReadFile(metaPath)
		if err != nil {
			// Not a claudia plugin — skip silently.
			continue
		}

		var meta metadata.Metadata
		if err := json.Unmarshal(data, &meta); err != nil {
			return nil, fmt.Errorf("parse metadata.json in %s: %w", dir, err)
		}

		entries = append(entries, Entry{
			Name:      meta.Name,
			Version:   meta.Version,
			SHA:       shortSHA(meta.GitCommitSHA),
			Installed: formatDate(meta.BuildTimestamp),
			Dir:       dir,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}

// shortSHA returns the first 7 characters of a git commit SHA.
func shortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	return sha
}

// formatDate trims an RFC3339 timestamp to its date portion (YYYY-MM-DD).
func formatDate(ts string) string {
	if idx := strings.IndexByte(ts, 'T'); idx > 0 {
		return ts[:idx]
	}
	return ts
}
