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

	"github.com/spf13/cobra"

	"github.com/retr0h/agentpack/pkg/cli"
	"github.com/retr0h/agentpack/pkg/list"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed agentpack plugins",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		out := cmd.OutOrStdout()

		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("home dir: %w", err)
		}
		pluginDir := filepath.Join(home, ".claude", "plugins")

		entries, err := list.Run(pluginDir)
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			cli.Print(out, "no plugins installed")
			return nil
		}

		// Calculate column widths.
		nameW := len("NAME")
		versionW := len("VERSION")
		shaW := len("SHA")
		dateW := len("INSTALLED")

		for _, e := range entries {
			if len(e.Name) > nameW {
				nameW = len(e.Name)
			}
			if len(e.Version) > versionW {
				versionW = len(e.Version)
			}
			if len(e.SHA) > shaW {
				shaW = len(e.SHA)
			}
			if len(e.Installed) > dateW {
				dateW = len(e.Installed)
			}
		}

		const pad = 2

		hdr := cli.Pad("NAME", nameW+pad) +
			cli.Pad("VERSION", versionW+pad) +
			cli.Pad("SHA", shaW+pad) +
			"INSTALLED"
		cli.Printf(out, "%s\n", cli.Mute(out, hdr))

		for _, e := range entries {
			name := cli.Accent(out, cli.Pad(e.Name, nameW+pad))
			version := cli.Pad(e.Version, versionW+pad)
			sha := cli.Mute(out, cli.Pad(e.SHA, shaW+pad))
			installed := cli.Info(out, e.Installed)
			cli.Printf(out, "%s%s%s%s\n", name, version, sha, installed)
		}

		cli.Printf(
			out, "\n%d %s installed\n",
			len(entries),
			plural(len(entries), "plugin", "plugins"),
		)

		return nil
	},
}

// plural returns singular when n == 1, otherwise plural.
func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}

func init() {
	rootCmd.AddCommand(listCmd)
}
