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

	"github.com/spf13/cobra"

	"github.com/retr0h/agentpack/internal/cli"
	"github.com/retr0h/agentpack/pkg/install"
	pkgupdate "github.com/retr0h/agentpack/pkg/update"
)

type updater interface {
	Run(ctx context.Context, opts pkgupdate.Options) (*pkgupdate.Result, error)
}

var pkgUpdater updater = pkgupdate.New()

var updateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "Update an installed agentpack plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		out := cmd.OutOrStdout()

		var onStep func(install.Step)
		if outputFormat != "json" {
			cli.Header(out, "updating", args[0])
			onStep = func(s install.Step) {
				cli.StepLine(out, s.Name, s.Detail)
			}
		}

		result, err := pkgUpdater.Run(ctx, pkgupdate.Options{
			Name:   args[0],
			OnStep: onStep,
		})
		if err != nil {
			return err
		}

		if outputFormat == "json" {
			return jsonOutput(out, result)
		}

		if result.Updated {
			cli.Printf(
				out, "  %s %s → %s\n",
				cli.OK(out, "updated"),
				cli.Mute(out, result.OldSHA),
				cli.Accent(out, result.NewSHA),
			)
		} else {
			cli.Printf(
				out, "  %s %s\n",
				cli.OK(out, cli.Checkmark),
				cli.Mute(out, "already up to date"),
			)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
