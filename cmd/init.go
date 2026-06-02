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

	"github.com/retr0h/agentpack/internal/cli"
	"github.com/retr0h/agentpack/internal/initpkg"
)

// scaffolder is the consumer-side interface for pkg/initpkg.Scaffold.
type scaffolder interface {
	Run(opts initpkg.Options) error
}

var pkgScaffolder scaffolder = initpkg.New()

// initResult holds the JSON-serialisable outcome of an init run.
type initResult struct {
	Name string `json:"name"`
	Dir  string `json:"dir"`
}

var initCmd = &cobra.Command{
	Use:   "init [name]",
	Short: "Initialize a new skill project",
	Long: `Scaffold a new skill project with a SKILL.md template and agentpack.yaml.

When a name argument is given, a new subdirectory with that name is created in
the current directory and the project is scaffolded inside it.

Without a name argument, the current directory name is used as the skill name
and the project is scaffolded in-place.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()

		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getwd: %w", err)
		}

		var name, dir string

		if len(args) == 1 {
			name = args[0]
			dir = filepath.Join(cwd, name)
		} else {
			name = filepath.Base(cwd)
			dir = cwd
		}

		if err := pkgScaffolder.Run(initpkg.Options{Name: name, Dir: dir}); err != nil {
			return err
		}

		result := initResult{Name: name, Dir: dir}

		if outputFormat == "json" {
			return jsonOutput(out, result)
		}

		cli.Printf(
			out,
			"%s %s\n\n  %s %s\n  %s %s\n\n",
			cli.Mute(out, "agentpack: init"),
			cli.Accent(out, name),
			cli.Mute(out, "name:"),
			name,
			cli.Mute(out, "dir: "),
			cli.Mute(out, dir),
		)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
