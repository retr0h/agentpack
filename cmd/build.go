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

	"github.com/avfs/avfs/vfs/osfs"
	"github.com/spf13/cobra"

	"github.com/retr0h/agentpack/internal/cli"
	"github.com/retr0h/agentpack/pkg/build"
)

var buildCmd = &cobra.Command{
	Use:   "build [plugin-names...]",
	Short: "Build .agentpack archives from a agentpack.yaml manifest",
	Long: `Build checksummed .agentpack archives for one or more plugins defined in
agentpack.yaml. When plugin names are given as arguments, only those plugins
are built. Otherwise all plugins in the manifest are built.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		dir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getwd: %w", err)
		}

		vfs := osfs.NewWithNoIdm()
		out := cmd.OutOrStdout()

		results, err := build.Run(ctx, vfs, build.Options{Dir: dir, Names: args})
		if err != nil {
			return err
		}

		for _, r := range results {
			cli.Printf(
				out,
				"%s %s %s\n\n  %s  (%s)\n  sha256: %s\n\n",
				cli.Mute(out, "agentpack: building"),
				cli.Accent(out, r.Name),
				cli.Mute(out, "v"+r.Version),
				filepath.Base(r.ArchivePath),
				cli.Mute(out, cli.HumanSize(r.Size)),
				cli.Mute(out, r.SHA256),
			)
		}

		if len(results) > 1 {
			cli.Printf(out, "\n%d archives built\n", len(results))
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)
}
