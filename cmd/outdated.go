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

	"github.com/retr0h/agentpack/pkg/cli"
	pkgoutdated "github.com/retr0h/agentpack/pkg/outdated"
)

var outdatedCmd = &cobra.Command{
	Use:   "outdated [names...]",
	Short: "Check for newer versions of installed plugins",
	Long:  `Outdated compares the installed SHA against the remote HEAD for each plugin without cloning. Optionally check only the named plugins.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		out := cmd.OutOrStdout()

		cli.Printf(out, "%s\n\n", cli.Mute(out, "agentpack: checking for updates"))

		entries, err := pkgoutdated.RunWithOptions(ctx, pkgoutdated.Options{
			Names: args,
			OnStep: func(name string) {
				cli.Printf(out, "  %s %s\n",
					cli.Mute(out, "checking"),
					cli.Mute(out, name),
				)
			},
		})
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			cli.Print(out, "all plugins up to date")

			return nil
		}

		cli.Print(out, "")

		for _, e := range entries {
			if e.Outdated {
				cli.Printf(
					out,
					"  %s  %s → %s\n",
					cli.Accent(out, e.Name),
					cli.Mute(out, shortSHA7(e.InstalledSHA)),
					cli.Mute(out, shortSHA7(e.RemoteSHA)),
				)
			} else {
				cli.Printf(
					out,
					"  %s  %s\n",
					cli.Mute(out, e.Name),
					cli.OK(out, "up to date"),
				)
			}
		}

		return nil
	},
}

// shortSHA7 returns the first 7 characters of sha, or the full string if shorter.
func shortSHA7(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}

	return sha
}

func init() {
	rootCmd.AddCommand(outdatedCmd)
}
