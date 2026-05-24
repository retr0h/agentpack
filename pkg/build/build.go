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

// Package build orchestrates the agentpack build pipeline.
package build

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"

	"github.com/avfs/avfs"
	"gopkg.in/yaml.v3"

	"github.com/retr0h/agentpack/pkg/archive"
	"github.com/retr0h/agentpack/pkg/checksum"
	"github.com/retr0h/agentpack/pkg/manifest"
	"github.com/retr0h/agentpack/pkg/metadata"
	"github.com/retr0h/agentpack/pkg/plugin"
)

// Options configures a build run.
type Options struct {
	Dir   string   // working directory (must be a git repo)
	Names []string // plugin names to build (empty = all)
}

// Result holds the outcome of building a single plugin.
type Result struct {
	Name        string
	Version     string
	ArchivePath string
	SHA256      string
	FileCount   int
	Size        int64
}

// Run executes the build pipeline for all selected plugins.
func Run(ctx context.Context, vfs avfs.VFS, opts Options) ([]Result, error) {
	m, err := manifest.Load(ctx, vfs, opts.Dir)
	if err != nil {
		return nil, err
	}

	plugins := manifest.Normalize(m)

	if len(opts.Names) > 0 {
		plugins, err = filterPlugins(plugins, opts.Names)
		if err != nil {
			return nil, err
		}
	}

	meta, err := metadata.Capture(ctx, opts.Dir, "", "")
	if err != nil {
		return nil, err
	}

	var results []Result

	for _, p := range plugins {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		r, err := buildPlugin(ctx, vfs, opts.Dir, p, meta)
		if err != nil {
			return nil, fmt.Errorf("plugin %q: %w", p.Name, err)
		}

		results = append(results, r)
	}

	return results, nil
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
			return nil, fmt.Errorf("plugin %q not found in agentpack.yaml", n)
		}
		result = append(result, p)
	}

	return result, nil
}

func buildPlugin(
	ctx context.Context,
	vfs avfs.VFS,
	dir string,
	p manifest.Plugin,
	meta *metadata.Metadata,
) (Result, error) {
	meta.Name = p.Name
	meta.Version = p.Version

	prefix := path.Join("marketplaces", p.Name) + "/"

	var files []archive.FileEntry

	type section struct {
		label   string
		entries []manifest.Entry
	}

	sections := []section{
		{"skills", p.Skills},
		{"commands", p.Commands},
		{"hooks", p.Hooks},
		{"agents", p.Agents},
		{"binaries", p.Binaries},
		{"settings", p.Settings},
	}

	var commandDests []string

	for _, s := range sections {
		if len(s.entries) == 0 {
			continue
		}

		pairs, err := manifest.ResolveEntries(ctx, vfs, dir, s.entries)
		if err != nil {
			return Result{}, fmt.Errorf("%s: %w", s.label, err)
		}

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

	mcpFiles, _, err := buildMCPEntries(ctx, vfs, dir, p, prefix)
	if err != nil {
		return Result{}, err
	}
	files = append(files, mcpFiles...)

	marketplaceJSON, err := plugin.GenerateMarketplace(p)
	if err != nil {
		return Result{}, fmt.Errorf("generating marketplace.json: %w", err)
	}
	files = append(files, archive.FileEntry{
		ArchivePath: path.Join(prefix, ".claude-plugin/marketplace.json"),
		Content:     marketplaceJSON,
	})

	pluginJSON, err := plugin.GeneratePlugin(p, commandDests)
	if err != nil {
		return Result{}, fmt.Errorf("generating plugin.json: %w", err)
	}
	files = append(files, archive.FileEntry{
		ArchivePath: path.Join(prefix, ".claude-plugin/plugin.json"),
		Content:     pluginJSON,
	})

	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return Result{}, fmt.Errorf("marshaling metadata: %w", err)
	}
	files = append(files, archive.FileEntry{
		ArchivePath: path.Join(prefix, ".agentpack/metadata.json"),
		Content:     metaJSON,
	})

	manifestYAML, err := yaml.Marshal(p)
	if err != nil {
		return Result{}, fmt.Errorf("marshaling agentpack.yaml: %w", err)
	}
	files = append(files, archive.FileEntry{
		ArchivePath: path.Join(prefix, ".agentpack/agentpack.yaml"),
		Content:     manifestYAML,
	})

	checksumEntries, err := computeArchiveChecksums(ctx, vfs, files)
	if err != nil {
		return Result{}, err
	}
	checksumContent := formatChecksums(checksumEntries)
	files = append(files, archive.FileEntry{
		ArchivePath: path.Join(prefix, ".agentpack/checksums.txt"),
		Content:     checksumContent,
	})

	outputName := fmt.Sprintf("%s-%s.agentpack", p.Name, p.Version)
	outputPath := filepath.Join(dir, outputName)

	if err := archive.Create(ctx, vfs, outputPath, files); err != nil {
		return Result{}, fmt.Errorf("creating archive: %w", err)
	}

	info, err := vfs.Stat(outputPath)
	if err != nil {
		return Result{}, fmt.Errorf("stat archive: %w", err)
	}

	archiveHash, err := checksum.ComputeFile(ctx, vfs, outputPath)
	if err != nil {
		return Result{}, fmt.Errorf("hashing archive: %w", err)
	}

	return Result{
		Name:        p.Name,
		Version:     p.Version,
		ArchivePath: outputPath,
		SHA256:      archiveHash,
		FileCount:   len(checksumEntries),
		Size:        info.Size(),
	}, nil
}

func buildMCPEntries(
	ctx context.Context,
	vfs avfs.VFS,
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
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}

		if mcp.Type == "binary" && mcp.Src != "" {
			srcPath := filepath.Join(dir, mcp.Src)
			if _, err := vfs.Stat(srcPath); err != nil {
				if avfs.IsNotExist(err) {
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
			if _, err := vfs.Stat(srcPath); err != nil {
				if avfs.IsNotExist(err) {
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

func computeArchiveChecksums(
	ctx context.Context,
	vfs avfs.VFS,
	files []archive.FileEntry,
) ([]checksum.Entry, error) {
	var entries []checksum.Entry

	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		var hash string

		if f.Src != "" {
			var err error
			hash, err = checksum.ComputeFile(ctx, vfs, f.Src)
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
