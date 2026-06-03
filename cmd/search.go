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
	"github.com/retr0h/agentpack/internal/search"
)

type searcher interface {
	Run(ctx context.Context, opts search.Options) ([]search.Result, error)
}

var pkgSearcher searcher = search.New()

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search for skills on agentskills.io",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		out := cmd.OutOrStdout()

		query := ""
		if len(args) > 0 {
			query = args[0]
		}

		results, err := pkgSearcher.Run(ctx, search.Options{Query: query})
		if err != nil {
			return err
		}

		if outputFormat == "json" {
			return jsonOutput(out, results)
		}

		if len(results) == 0 {
			cli.Print(out, "no skills found")
			return nil
		}

		maxName := 0
		maxSource := 0
		for _, r := range results {
			if len(r.Name) > maxName {
				maxName = len(r.Name)
			}
			if len(r.Source) > maxSource {
				maxSource = len(r.Source)
			}
		}

		cli.Printf(
			out, "\n%s %s\n\n",
			cli.Mute(out, "Install with"),
			"agentpack add <source:skill/name>",
		)
		for _, r := range results {
			installCmd := r.Source + ":skill/" + r.Name
			installs := cli.Info(out, cli.FormatInstalls(r.Installs))
			link := cli.Mute(out, "https://skills.sh/"+r.Source+"/"+r.Name)
			cli.Printf(
				out,
				"%s %s\n%s %s\n\n",
				cli.Accent(out, installCmd),
				installs,
				cli.Mute(out, "└"),
				link,
			)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
}
