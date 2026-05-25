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
	"github.com/retr0h/agentpack/pkg/list"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed agentpack plugins",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		out := cmd.OutOrStdout()

		entries, err := list.Run()
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			cli.Print(out, "no plugins installed")

			return nil
		}

		names := make([]string, len(entries))
		versions := make([]string, len(entries))
		shas := make([]string, len(entries))
		sources := make([]string, len(entries))
		targets := make([]string, len(entries))
		installed := make([]string, len(entries))

		for i, e := range entries {
			names[i] = e.Name
			versions[i] = e.Version
			shas[i] = e.SHA
			sources[i] = e.Source
			targets[i] = e.Targets
			installed[i] = e.Installed
		}

		cli.Table(out, []cli.TableColumn{
			{Header: "NAME", Values: names, Accent: true},
			{Header: "VERSION", Values: versions},
			{Header: "SHA", Values: shas, Muted: true},
			{Header: "SOURCE", Values: sources, Muted: true},
			{Header: "TARGETS", Values: targets, Muted: true},
			{Header: "INSTALLED", Values: installed, Info: true},
		})

		cli.Printf(
			out, "\n%d %s installed\n",
			len(entries),
			cli.Plural(len(entries), "plugin", "plugins"),
		)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
