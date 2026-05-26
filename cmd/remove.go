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
	"github.com/spf13/cobra"

	"github.com/retr0h/agentpack/internal/cli"
	pkgremove "github.com/retr0h/agentpack/pkg/remove"
)

var removeCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an installed agentpack plugin",
	Long: `Remove an installed agentpack plugin. Only the exact files recorded in
the plugin registry are deleted. User-modified files are skipped. The .git
directory is never touched.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		out := cmd.OutOrStdout()

		name := args[0]

		cli.Header(out, "removing", name)

		result, err := pkgremove.Run(ctx, pkgremove.Options{
			Name: name,
			OnStep: func(s pkgremove.Step) {
				if s.Skipped {
					cli.Printf(out, "  %s %s\n", cli.Mute(out, "skipped"), cli.Mute(out, s.Path))
				} else {
					cli.StepLine(out, "removed", s.Path)
				}
			},
		})
		if err != nil {
			return err
		}

		cli.Printf(out, "\n  %s removed\n", cli.OK(out, result.Name))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(removeCmd)
}
