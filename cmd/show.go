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
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/retr0h/agentpack/internal/cli"
	"github.com/retr0h/agentpack/internal/safety"
	"github.com/retr0h/agentpack/pkg/inspect"
	"github.com/retr0h/agentpack/pkg/registry"
)

type registryLoader interface {
	Load(name string) (*registry.PackageManifest, error)
}

type inspector interface {
	Run(ctx context.Context, opts inspect.Options) (*inspect.Result, error)
}

var (
	pkgRegistryLoader registryLoader = registry.New()
	pkgInspector      inspector      = inspect.New()
	infoGlobal        bool
)

func isArchiveFile(arg string) bool {
	return strings.HasSuffix(arg, ".agentpack")
}

var infoCmd = &cobra.Command{
	Use:   "info <name | archive.agentpack>",
	Short: "Show details of an installed package or archive contents",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if isArchiveFile(args[0]) {
			return showArchive(cmd, args[0])
		}

		return showInstalled(cmd, args[0])
	},
}

func showInstalled(cmd *cobra.Command, name string) error {
	out := cmd.OutOrStdout()

	m, err := pkgRegistryLoader.Load(name)
	if err != nil {
		return err
	}

	if outputFormat == "json" {
		return jsonOutput(out, m)
	}

	installed := m.Installed
	if idx := strings.IndexByte(installed, 'T'); idx > 0 {
		installed = installed[:idx]
	}

	version := m.Version
	if len(version) >= 40 {
		version = cli.ShortSHA(version)
	}

	source := m.Source
	source = strings.TrimPrefix(source, "https://")
	source = strings.TrimPrefix(source, "http://")
	if idx := strings.IndexByte(source, '#'); idx >= 0 {
		source = source[:idx]
	}

	cli.FieldAccent(out, "Name", m.Name)
	cli.Field(out, "Version", version)
	cli.FieldMuted(out, "Source", source)
	cli.FieldMuted(out, "SHA", cli.ShortSHA(m.SHA))
	cli.FieldInfo(out, "Installed", installed)
	if infoGlobal {
		cli.Field(out, "Scope", "global")
	}

	archiveBase := fmt.Sprintf("%s@%s", m.Name, cli.ShortSHA(m.SHA))
	archivePath := fmt.Sprintf("~/.config/agentpack/archives/%s.agentpack", archiveBase)
	cli.Field(out, "Archive", archivePath)

	shaFilePath := fmt.Sprintf("~/.config/agentpack/archives/%s.sha256", archiveBase)
	cli.Field(out, "SHA256", shaFilePath)

	type targetGroup struct {
		dir   string
		files []registry.InstalledFile
	}

	groups := make(map[string]*targetGroup)
	var order []string

	for _, f := range m.Files {
		g, ok := groups[f.Target]
		if !ok {
			g = &targetGroup{dir: f.Dir}
			groups[f.Target] = g
			order = append(order, f.Target)
		}

		g.files = append(g.files, f)
	}

	for _, tgt := range order {
		g := groups[tgt]
		cli.Printf(
			out, "\n%s %s\n",
			cli.Tag(out, tgt),
			cli.Mute(out, fmt.Sprintf("(%s, %d files)", g.dir, len(g.files))),
		)

		for _, f := range g.files {
			path := f.Path
			if safety.IsExecutable(path) {
				path = cli.Err(out, path)
			}
			cli.Printf(
				out, "  %s  %s\n",
				path,
				cli.Mute(out, cli.ShortSHA(f.SHA256)),
			)
		}
	}

	return nil
}

func showArchive(cmd *cobra.Command, path string) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	result, err := pkgInspector.Run(ctx, inspect.Options{Path: path})
	if err != nil {
		return err
	}

	if outputFormat == "json" {
		return jsonOutput(out, result)
	}

	built := result.Built
	if idx := strings.IndexByte(built, 'T'); idx > 0 {
		built = built[:idx]
	}

	cli.FieldAccent(out, "Package", result.Name)
	cli.Field(out, "Version", result.Version)
	cli.FieldInfo(out, "Built", built)
	cli.FieldMuted(out, "SHA", cli.ShortSHA(result.SHA))

	if result.Content != nil {
		safeCount := len(result.Content.Safe)
		execCount := len(result.Content.Executable)
		contentSummary := fmt.Sprintf("%d safe, %d executable", safeCount, execCount)
		cli.Field(out, "Content", contentSummary)

		for _, f := range result.Content.Executable {
			cli.Printf(out, "  %s %s\n", cli.Err(out, "!"), cli.Mute(out, f))
		}
	}

	cli.Printf(out, "\n%s\n", cli.Mute(out, "Contents:"))

	maxPath := 0
	for _, f := range result.Files {
		if len(f.Path) > maxPath {
			maxPath = len(f.Path)
		}
	}

	for _, f := range result.Files {
		var mark string
		if f.Verified {
			mark = cli.OK(out, cli.Checkmark)
		} else {
			mark = cli.Err(out, "✗")
		}

		padded := cli.Pad(f.Path, maxPath)
		if safety.IsExecutable(f.Path) {
			padded = cli.Err(out, padded)
		}
		size := humanSize(f.Size)
		short := cli.ShortSHA(f.SHA256)

		cli.Printf(
			out, "  %s %s  %s  %s\n",
			mark,
			padded,
			cli.Mute(out, fmt.Sprintf("%8s", size)),
			cli.Mute(out, short),
		)
	}

	cli.Printf(
		out, "\n%s\n",
		cli.Mute(out, fmt.Sprintf(
			"%d %s, %s total",
			len(result.Files),
			cli.Plural(len(result.Files), "file", "files"),
			humanSize(result.Total),
		)),
	)

	return nil
}

func humanSize(bytes int64) string {
	const kb = 1024
	if bytes < kb {
		return fmt.Sprintf("%d B", bytes)
	}

	return fmt.Sprintf("%.1f KB", float64(bytes)/kb)
}

func init() {
	rootCmd.AddCommand(infoCmd)

	infoCmd.Flags().
		BoolVarP(&infoGlobal, "global", "g", false, "show global install scope indicator")
}
