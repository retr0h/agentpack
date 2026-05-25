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
	"strings"

	"github.com/spf13/cobra"

	"github.com/retr0h/agentpack/pkg/cli"
	"github.com/retr0h/agentpack/pkg/registry"
)

var showCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show details of an installed package",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		name := args[0]

		m, err := registry.Load(name)
		if err != nil {
			return err
		}

		installed := m.Installed
		if idx := strings.IndexByte(installed, 'T'); idx > 0 {
			installed = installed[:idx]
		}

		cli.Printf(out, "%s %s\n", cli.Mute(out, "Name:"), cli.Accent(out, m.Name))
		cli.Printf(out, "%s %s\n", cli.Mute(out, "Version:"), cli.Accent(out, m.Version))
		cli.Printf(out, "%s %s\n", cli.Mute(out, "Source:"), cli.Accent(out, m.Source))
		cli.Printf(out, "%s %s\n", cli.Mute(out, "SHA:"), cli.Accent(out, shortSHAShow(m.SHA)))
		cli.Printf(out, "%s %s\n", cli.Mute(out, "Installed:"), cli.Accent(out, installed))

		archivePath := fmt.Sprintf("~/.config/agentpack/archives/%s@%s.agentpack", m.Name, shortSHAShow(m.SHA))
		cli.Printf(out, "%s %s\n", cli.Mute(out, "Archive:"), cli.Mute(out, archivePath))
		// Group files by target to show base dir once per target.
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
			cli.Printf(out, "\n%s %s\n",
				cli.Accent(out, tgt),
				cli.Mute(out, fmt.Sprintf("(%s, %d files)", g.dir, len(g.files))),
			)

			for _, f := range g.files {
				cli.Printf(out, "  %s  %s\n",
					cli.Mute(out, f.Path),
					cli.Mute(out, shortSHAShow(f.SHA256)),
				)
			}
		}

		return nil
	},
}

// shortSHAShow truncates a hex string to 7 characters for display.
func shortSHAShow(s string) string {
	if len(s) >= 7 {
		return s[:7]
	}

	return s
}

func init() {
	rootCmd.AddCommand(showCmd)
}
