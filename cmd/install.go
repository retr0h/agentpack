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

	"github.com/spf13/cobra"

	"github.com/retr0h/agentpack/internal/cli"
	"github.com/retr0h/agentpack/pkg/install"
)

type installer interface {
	Run(ctx context.Context, opts install.Options) (*install.Result, error)
}

type defaultInstaller struct{}

func (defaultInstaller) Run(ctx context.Context, opts install.Options) (*install.Result, error) {
	return install.Run(ctx, opts)
}

var pkgInstaller installer = defaultInstaller{}

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
		displayName := cli.SourceBaseName(source)

		cli.Header(out, "installing", displayName)

		result, err := pkgInstaller.Run(ctx, install.Options{
			Source: source,
			Skills: installSkills,
			Agents: installAgents,
			OnStep: func(s install.Step) {
				cli.StepLine(out, s.Name, s.Detail)
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
			cli.TreeRow(out, i == len(rows)-1, row.targetName, 12, fmt.Sprintf("(%d %s)", row.count, cli.Plural(row.count, "file", "files")))
		}

		cli.Printf(out, "\n  %s %s %s\n", cli.OK(out, cli.Checkmark), cli.Accent(out, result.Name), cli.Mute(out, "installed"))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(installCmd)

	installCmd.Flags().StringArrayVar(&installSkills, "skill", nil, "install only specific skill(s) by name (may be repeated)")
	installCmd.Flags().StringArrayVar(&installAgents, "agent", nil, "install only specific agent(s) by name (may be repeated)")
}
