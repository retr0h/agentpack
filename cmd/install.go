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
	"github.com/retr0h/agentpack/pkg/install"
)

var installCmd = &cobra.Command{
	Use:   "install <source>",
	Short: "Install a .agentpack archive",
	Long: `Install a .agentpack archive into the Claude Code plugin directory.
Source may be a local file path or an HTTP/HTTPS URL.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		out := cmd.OutOrStdout()

		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("home dir: %w", err)
		}
		pluginDir := filepath.Join(home, ".claude", "plugins")

		result, err := install.Run(ctx, install.Options{
			Source:    args[0],
			PluginDir: pluginDir,
		})
		if err != nil {
			return err
		}

		cli.Printf(
			out,
			"%s %s %s (%s)\n\n",
			cli.Mute(out, "agentpack: installing"),
			cli.Accent(out, result.Name),
			cli.Mute(out, "v"+result.Version),
			cli.Mute(out, result.SHA),
		)

		cli.Printf(out, "  extracted to %s\n\n", cli.Mute(out, result.Dir))
		cli.Printf(out, "  %s installed\n", cli.OK(out, result.Name))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
}
