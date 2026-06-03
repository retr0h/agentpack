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
	pkgsync "github.com/retr0h/agentpack/pkg/sync"
)

type syncer interface {
	Run(ctx context.Context, opts pkgsync.Options) ([]pkgsync.Result, error)
}

var pkgSyncer syncer = pkgsync.New()

var syncConfigFlag string

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install plugins from agentpack-packages.yaml",
	Long: `Install reads agentpack-packages.yaml and installs every declared
plugin. When agentpack.lock exists, locked SHAs are used for reproducibility.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()
		out := cmd.OutOrStdout()

		var onStep func(string)
		if outputFormat != "json" {
			cli.Printf(
				out,
				"%s\n\n",
				cli.Mute(out, "agentpack: installing"),
			)
			onStep = func(name string) {
				cli.Printf(
					out, "  %s %s\n",
					cli.Mute(out, "installing"),
					cli.Accent(out, name),
				)
			}
		}

		results, err := pkgSyncer.Run(ctx, pkgsync.Options{
			ConfigPath: syncConfigFlag,
			Builder:    pkgsync.DefaultBuilder{},
			Installer:  pkgsync.DefaultInstaller{},
			OnStep:     onStep,
		})
		if err != nil {
			return err
		}

		if outputFormat == "json" {
			type syncResultJSON struct {
				Name    string `json:"name"`
				Version string `json:"version"`
				Status  string `json:"status"`
				Err     string `json:"error,omitempty"`
			}
			jsonResults := make([]syncResultJSON, len(results))
			for i, r := range results {
				jr := syncResultJSON{
					Name:    r.Name,
					Version: r.Version,
					Status:  string(r.Status),
				}
				if r.Err != nil {
					jr.Err = r.Err.Error()
				}
				jsonResults[i] = jr
			}
			return jsonOutput(out, jsonResults)
		}

		cli.Print(out, "")

		failed := 0

		for _, r := range results {
			switch r.Status {
			case pkgsync.StatusInstalled:
				cli.Printf(
					out, "  %s %s  %s\n",
					cli.OK(out, cli.Checkmark),
					cli.Accent(out, r.Name),
					r.Version,
				)
			case pkgsync.StatusUpToDate:
				cli.Printf(
					out, "  %s %s  %s\n",
					cli.OK(out, cli.Checkmark),
					r.Name,
					cli.OK(out, string(pkgsync.StatusUpToDate)),
				)
			case pkgsync.StatusFailed:
				cli.Printf(
					out, "  %s %s  %s\n",
					cli.Err(out, "✗"),
					cli.Accent(out, r.Name),
					cli.Err(out, r.Err.Error()),
				)
				failed++
			}
		}

		cli.Printf(
			out, "\n%d %s installed\n",
			len(results),
			cli.Plural(len(results), "plugin", "plugins"),
		)

		if failed > 0 {
			return fmt.Errorf("%d package(s) failed to install", failed)
		}

		return nil
	},
}

func init() {
	installCmd.Flags().StringVarP(
		&syncConfigFlag,
		"config", "c",
		"agentpack-packages.yaml",
		"path to agentpack-packages.yaml",
	)
	rootCmd.AddCommand(installCmd)
}
