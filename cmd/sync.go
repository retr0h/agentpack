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

	"github.com/spf13/cobra"

	"github.com/retr0h/agentpack/internal/cli"
	pkgsync "github.com/retr0h/agentpack/pkg/sync"
)

var syncConfigFlag string

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync plugins from agentpack-packages.yaml",
	Long: `Sync reads agentpack-packages.yaml and installs or updates every declared
plugin into the Claude Code plugin directory.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()
		out := cmd.OutOrStdout()

		cli.Printf(
			out,
			"%s\n\n",
			cli.Mute(out, "agentpack: syncing"),
		)

		results, err := pkgsync.Run(ctx, pkgsync.Options{
			ConfigPath: syncConfigFlag,
			Builder:    pkgsync.DefaultBuilder{},
			Installer:  pkgsync.DefaultInstaller{},
			OnStep: func(name string) {
				cli.Printf(out, "  %s %s\n",
					cli.Mute(out, "syncing"),
					cli.Accent(out, name),
				)
			},
		})
		if err != nil {
			return err
		}

		cli.Print(out, "")

		failed := 0

		for _, r := range results {
			switch r.Status {
			case "installed":
				cli.Printf(
					out, "  %s %s  %s\n",
					cli.OK(out, cli.Checkmark),
					cli.Accent(out, r.Name),
					cli.Mute(out, r.Version),
				)
			case "up to date":
				cli.Printf(
					out, "  %s %s  %s\n",
					cli.OK(out, cli.Checkmark),
					cli.Mute(out, r.Name),
					cli.Mute(out, "up to date"),
				)
			case "failed":
				cli.Printf(
					out, "  %s %s  %s\n",
					cli.Err(out, "✗"),
					cli.Accent(out, r.Name),
					cli.Mute(out, r.Err.Error()),
				)
				failed++
			}
		}

		cli.Printf(
			out, "\n%d %s synced\n",
			len(results),
			cli.Plural(len(results), "plugin", "plugins"),
		)

		if failed > 0 {
			return fmt.Errorf("%d package(s) failed to sync", failed)
		}

		return nil
	},
}

func init() {
	syncCmd.Flags().StringVarP(
		&syncConfigFlag,
		"config", "c",
		"agentpack-packages.yaml",
		"path to agentpack-packages.yaml",
	)
	rootCmd.AddCommand(syncCmd)
}
