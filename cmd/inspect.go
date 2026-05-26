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
	"github.com/retr0h/agentpack/pkg/inspect"
)

type inspector interface {
	Run(ctx context.Context, opts inspect.Options) (*inspect.Result, error)
}

var pkgInspector inspector = inspect.New()

var inspectCmd = &cobra.Command{
	Use:   "inspect <archive.agentpack>",
	Short: "Show contents of a .agentpack archive",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		out := cmd.OutOrStdout()

		result, err := pkgInspector.Run(ctx, inspect.Options{Path: args[0]})
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
	},
}

func humanSize(bytes int64) string {
	const kb = 1024
	if bytes < kb {
		return fmt.Sprintf("%d B", bytes)
	}
	return fmt.Sprintf("%.1f KB", float64(bytes)/kb)
}

func init() {
	rootCmd.AddCommand(inspectCmd)
}
