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
	"github.com/retr0h/agentpack/pkg/install"
)

// checkmark is the Unicode check character used in install output.
const checkmark = "✓"

var (
	installSkills []string
	installAgents []string
)

var installCmd = &cobra.Command{
	Use:   "install <source>",
	Short: "Install a .agentpack archive",
	Long: `Install a .agentpack archive into all detected AI coding agents.
Source may be a local file path or an HTTP/HTTPS URL.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		out := cmd.OutOrStdout()

		source := args[0]
		displayName := sourceBaseName(source)

		cli.Printf(out, "%s %s\n\n", cli.Mute(out, "agentpack: installing"), cli.Accent(out, displayName))

		result, err := install.Run(ctx, install.Options{
			Source: source,
			Skills: installSkills,
			Agents: installAgents,
			OnStep: func(s install.Step) {
				cli.Printf(out, "  %s %s %s\n",
					cli.OK(out, checkmark),
					cli.Mute(out, s.Name),
					cli.Mute(out, s.Detail),
				)
			},
		})
		if err != nil {
			return err
		}

		cli.Print(out, "")

		// Collect target names in deterministic order for consistent output.
		type targetRow struct {
			displayName string
			targetName  string
			count       int
		}

		var rows []targetRow

		for dn, tn := range result.Dirs {
			rows = append(rows, targetRow{displayName: dn, targetName: tn, count: result.FileCounts[dn]})
		}

		for i, row := range rows {
			prefix := "  ├─"
			if i == len(rows)-1 {
				prefix = "  └─"
			}

			cli.Printf(
				out,
				"%s %s  %s\n",
				cli.Mute(out, prefix),
				cli.Accent(out, cli.Pad(row.targetName, 12)),
				cli.Mute(out, fmt.Sprintf("(%d %s)", row.count, plural(row.count, "file", "files"))),
			)
		}

		cli.Printf(out, "\n  %s %s %s\n", cli.OK(out, checkmark), cli.Accent(out, result.Name), cli.Mute(out, "installed"))

		return nil
	},
}

// sourceBaseName extracts a short display name from a source path or URL.
func sourceBaseName(source string) string {
	s := source

	// Strip ref fragment (#branch or #sha).
	if idx := strings.LastIndex(s, "#"); idx >= 0 {
		s = s[:idx]
	}

	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimSuffix(s, "/")

	if idx := strings.LastIndex(s, "/"); idx >= 0 {
		return s[idx+1:]
	}

	return s
}

func init() {
	rootCmd.AddCommand(installCmd)

	installCmd.Flags().StringArrayVar(&installSkills, "skill", nil, "install only specific skill(s) by name (may be repeated)")
	installCmd.Flags().StringArrayVar(&installAgents, "agent", nil, "install only specific agent(s) by name (may be repeated)")
}
