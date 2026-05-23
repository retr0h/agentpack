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
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/retr0h/claudia/internal/archive"
	"github.com/retr0h/claudia/internal/checksum"
	"github.com/retr0h/claudia/internal/manifest"
	"github.com/retr0h/claudia/internal/metadata"
	"github.com/retr0h/claudia/internal/plugin"
)

var buildCmd = &cobra.Command{
	Use:   "build [plugin-names...]",
	Short: "Build .claudia archives from a claudia.yaml manifest",
	Long: `Build checksummed .claudia archives for one or more plugins defined in
claudia.yaml. When plugin names are given as arguments, only those plugins
are built. Otherwise all plugins in the manifest are built.`,
	RunE: func(_ *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getwd: %w", err)
		}
		return runBuild(dir, args)
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)
}

func runBuild(dir string, names []string) error {
	m, err := manifest.Load(dir)
	if err != nil {
		return err
	}

	plugins := manifest.Normalize(m)

	if len(names) > 0 {
		plugins, err = filterPlugins(plugins, names)
		if err != nil {
			return err
		}
	}

	meta, err := metadata.Capture(dir, "", "")
	if err != nil {
		return err
	}

	for _, p := range plugins {
		if err := buildPlugin(dir, p, meta); err != nil {
			return fmt.Errorf("plugin %q: %w", p.Name, err)
		}
	}

	if len(plugins) > 1 {
		fmt.Printf("\n%d archives built\n", len(plugins))
	}

	return nil
}

func filterPlugins(plugins []manifest.Plugin, names []string) ([]manifest.Plugin, error) {
	idx := make(map[string]manifest.Plugin, len(plugins))
	for _, p := range plugins {
		idx[p.Name] = p
	}

	result := make([]manifest.Plugin, 0, len(names))
	for _, n := range names {
		p, ok := idx[n]
		if !ok {
			return nil, fmt.Errorf("plugin %q not found in claudia.yaml", n)
		}
		result = append(result, p)
	}

	return result, nil
}

func buildPlugin(dir string, p manifest.Plugin, meta *metadata.Metadata) error {
	meta.Name = p.Name
	meta.Version = p.Version

	prefix := path.Join("marketplaces", p.Name) + "/"

	fmt.Printf("claudia: building %s v%s (%s)\n\n", p.Name, p.Version, shortSHA(meta.GitCommitSHA))

	var files []archive.FileEntry
	totalFiles := 0

	type section struct {
		label   string
		entries []manifest.Entry
		destDir string
	}

	sections := []section{
		{"skills", p.Skills, "skills"},
		{"commands", p.Commands, "commands"},
		{"hooks", p.Hooks, "hooks"},
		{"agents", p.Agents, "agents"},
		{"binaries", p.Binaries, "bin"},
		{"settings", p.Settings, "settings"},
	}

	var commandDests []string

	for _, s := range sections {
		if len(s.entries) == 0 {
			continue
		}

		pairs, err := manifest.ResolveEntries(dir, s.entries)
		if err != nil {
			return fmt.Errorf("%s: %w", s.label, err)
		}

		fmt.Printf("  %-14s %d files\n", s.label+"/", len(pairs))
		totalFiles += len(pairs)

		for _, fp := range pairs {
			files = append(files, archive.FileEntry{
				Src:         fp.Src,
				ArchivePath: path.Join(prefix, fp.Dest),
			})

			if s.label == "commands" {
				commandDests = append(commandDests, fp.Dest)
			}
		}
	}

	mcpFiles, mcpCount, err := buildMCPEntries(dir, p, prefix)
	if err != nil {
		return err
	}
	if mcpCount > 0 {
		fmt.Printf("  %-14s %d entries\n", "mcp/", mcpCount)
		totalFiles += mcpCount
		files = append(files, mcpFiles...)
	}

	marketplaceJSON, err := plugin.GenerateMarketplace(p)
	if err != nil {
		return fmt.Errorf("generating marketplace.json: %w", err)
	}
	files = append(files, archive.FileEntry{
		ArchivePath: path.Join(prefix, ".claude-plugin/marketplace.json"),
		Content:     marketplaceJSON,
	})
	totalFiles++

	pluginJSON, err := plugin.GeneratePlugin(p, commandDests)
	if err != nil {
		return fmt.Errorf("generating plugin.json: %w", err)
	}
	files = append(files, archive.FileEntry{
		ArchivePath: path.Join(prefix, ".claude-plugin/plugin.json"),
		Content:     pluginJSON,
	})
	totalFiles++

	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}
	files = append(files, archive.FileEntry{
		ArchivePath: path.Join(prefix, ".claudia/metadata.json"),
		Content:     metaJSON,
	})
	totalFiles++

	manifestYAML, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshaling claudia.yaml: %w", err)
	}
	files = append(files, archive.FileEntry{
		ArchivePath: path.Join(prefix, ".claudia/claudia.yaml"),
		Content:     manifestYAML,
	})
	totalFiles++

	checksumEntries, err := computeArchiveChecksums(files)
	if err != nil {
		return err
	}
	checksumContent := formatChecksums(checksumEntries)
	files = append(files, archive.FileEntry{
		ArchivePath: path.Join(prefix, ".claudia/checksums.txt"),
		Content:     checksumContent,
	})

	fmt.Printf("\n  checksummed %d files (SHA256)\n", len(checksumEntries))

	outputName := fmt.Sprintf("%s-%s.claudia", p.Name, p.Version)
	outputPath := filepath.Join(dir, outputName)

	if err := archive.Create(outputPath, files); err != nil {
		return fmt.Errorf("creating archive: %w", err)
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		return fmt.Errorf("stat archive: %w", err)
	}

	archiveHash, err := checksum.ComputeFile(outputPath)
	if err != nil {
		return fmt.Errorf("hashing archive: %w", err)
	}

	fmt.Printf("\n  %s  (%s)\n", outputName, humanSize(info.Size()))
	fmt.Printf("  sha256: %s\n\n", archiveHash)

	return nil
}

func buildMCPEntries(
	dir string,
	p manifest.Plugin,
	prefix string,
) ([]archive.FileEntry, int, error) {
	if len(p.MCP) == 0 {
		return nil, 0, nil
	}

	var files []archive.FileEntry
	count := 0

	for _, mcp := range p.MCP {
		if mcp.Type == "binary" && mcp.Src != "" {
			srcPath := filepath.Join(dir, mcp.Src)
			if _, err := os.Stat(srcPath); err != nil {
				if os.IsNotExist(err) {
					return nil, 0, fmt.Errorf("mcp binary not found: %s", mcp.Src)
				}
				return nil, 0, fmt.Errorf("stat mcp binary: %w", err)
			}
			files = append(files, archive.FileEntry{
				Src:         srcPath,
				ArchivePath: path.Join(prefix, "mcp", filepath.Base(mcp.Src)),
			})
			count++
		}

		if mcp.Config != "" {
			srcPath := filepath.Join(dir, mcp.Config)
			if _, err := os.Stat(srcPath); err != nil {
				if os.IsNotExist(err) {
					return nil, 0, fmt.Errorf("mcp config not found: %s", mcp.Config)
				}
				return nil, 0, fmt.Errorf("stat mcp config: %w", err)
			}
			files = append(files, archive.FileEntry{
				Src:         srcPath,
				ArchivePath: path.Join(prefix, "mcp/.mcp.json"),
			})
			count++
		}
	}

	var generatable []manifest.MCPEntry
	for _, mcp := range p.MCP {
		if mcp.Config == "" {
			generatable = append(generatable, mcp)
		}
	}

	mcpJSON, err := plugin.GenerateMCPConfig(generatable)
	if err != nil {
		return nil, 0, fmt.Errorf("generating .mcp.json: %w", err)
	}
	if mcpJSON != nil {
		files = append(files, archive.FileEntry{
			ArchivePath: path.Join(prefix, "mcp/.mcp.json"),
			Content:     mcpJSON,
		})
		count++
	}

	return files, count, nil
}

func computeArchiveChecksums(files []archive.FileEntry) ([]checksum.Entry, error) {
	var entries []checksum.Entry

	for _, f := range files {
		var hash string

		if f.Src != "" {
			var err error
			hash, err = checksum.ComputeFile(f.Src)
			if err != nil {
				return nil, fmt.Errorf("checksum %s: %w", f.Src, err)
			}
		} else {
			hash = checksum.ComputeBytes(f.Content)
		}

		entries = append(entries, checksum.Entry{
			Hash: hash,
			Path: f.ArchivePath,
		})
	}

	return entries, nil
}

func formatChecksums(entries []checksum.Entry) []byte {
	var buf []byte
	for _, e := range entries {
		buf = fmt.Appendf(buf, "%s  %s\n", e.Hash, e.Path)
	}
	return buf
}

func shortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	return sha
}

func humanSize(bytes int64) string {
	const kb = 1024
	if bytes < kb {
		return fmt.Sprintf("%d B", bytes)
	}
	return fmt.Sprintf("%d KB", bytes/kb)
}
