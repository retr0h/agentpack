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

	"github.com/retr0h/claudia/pkg/build"
)

var buildCmd = &cobra.Command{
	Use:   "build [plugin-names...]",
	Short: "Build .claudia archives from a claudia.yaml manifest",
	Long: `Build checksummed .claudia archives for one or more plugins defined in
claudia.yaml. When plugin names are given as arguments, only those plugins
are built. Otherwise all plugins in the manifest are built.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		dir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getwd: %w", err)
		}

		vfs := osfs.NewWithNoIdm()

		results, err := build.Run(ctx, vfs, build.Options{Dir: dir, Names: args})
		if err != nil {
			return err
		}

		for _, r := range results {
			fmt.Fprintf(cmd.OutOrStdout(),
				"claudia: building %s v%s\n\n  %s  (%s)\n  sha256: %s\n\n",
				r.Name, r.Version,
				filepath.Base(r.ArchivePath), cmdHumanSize(r.Size),
				r.SHA256,
			)
		}

		if len(results) > 1 {
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d archives built\n", len(results))
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)
}

func cmdHumanSize(bytes int64) string {
	const kb = 1024
	if bytes < kb {
		return fmt.Sprintf("%d B", bytes)
	}
	return fmt.Sprintf("%d KB", bytes/kb)
}
