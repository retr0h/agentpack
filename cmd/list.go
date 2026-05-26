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
	"github.com/retr0h/agentpack/pkg/list"
	"github.com/retr0h/agentpack/pkg/outdated"
	"github.com/retr0h/agentpack/pkg/target"
)

type lister interface {
	Run() ([]list.Entry, error)
}

type outdatedChecker interface {
	RunWithOptions(ctx context.Context, opts outdated.Options) ([]outdated.Entry, error)
}

var (
	pkgLister          lister          = list.New()
	pkgOutdatedChecker outdatedChecker = outdated.New()
)

var (
	listOutdatedFlag bool
	listTargetsFlag  bool
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List installed agentpack plugins",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if listTargetsFlag {
			return listTargets(cmd)
		}

		if listOutdatedFlag {
			return listOutdated(cmd)
		}

		return listInstalled(cmd)
	},
}

func listInstalled(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()

	entries, err := pkgLister.Run()
	if err != nil {
		return err
	}

	if outputFormat == "json" {
		return jsonOutput(out, entries)
	}

	if len(entries) == 0 {
		cli.Print(out, "no plugins installed")

		return nil
	}

	names := make([]string, len(entries))
	versions := make([]string, len(entries))
	shas := make([]string, len(entries))
	targets := make([]string, len(entries))
	installed := make([]string, len(entries))
	sources := make([]string, len(entries))

	for i, e := range entries {
		names[i] = e.Name
		versions[i] = e.Version
		shas[i] = e.SHA
		targets[i] = e.Targets
		installed[i] = e.Installed
		sources[i] = e.Source
	}

	cli.Table(out, []cli.TableColumn{
		{Header: "NAME", Values: names, Accent: true},
		{Header: "VERSION", Values: versions},
		{Header: "SHA", Values: shas, Muted: true},
		{Header: "TARGETS", Values: targets, Tag: true},
		{Header: "INSTALLED", Values: installed, Info: true},
		{Header: "SOURCE", Values: sources, Muted: true},
	})

	cli.Printf(
		out, "\n%d %s installed\n",
		len(entries),
		cli.Plural(len(entries), "plugin", "plugins"),
	)

	return nil
}

func listOutdated(cmd *cobra.Command) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	var onStep func(string)
	if outputFormat != "json" {
		cli.Printf(out, "%s\n\n", cli.Mute(out, "agentpack: checking for updates"))
		onStep = func(name string) {
			cli.Printf(
				out, "  %s %s\n",
				cli.Mute(out, "checking"),
				cli.Mute(out, name),
			)
		}
	}

	entries, err := pkgOutdatedChecker.RunWithOptions(ctx, outdated.Options{
		OnStep: onStep,
	})
	if err != nil {
		return err
	}

	if outputFormat == "json" {
		return jsonOutput(out, entries)
	}

	if len(entries) == 0 {
		cli.Print(out, "all plugins up to date")

		return nil
	}

	cli.Print(out, "")

	for _, e := range entries {
		if e.Outdated {
			cli.Printf(
				out,
				"  %s  %s → %s\n",
				cli.Accent(out, e.Name),
				cli.Mute(out, cli.ShortSHA(e.InstalledSHA)),
				cli.ShortSHA(e.RemoteSHA),
			)
		} else {
			cli.Printf(
				out,
				"  %s  %s\n",
				e.Name,
				cli.OK(out, "up to date"),
			)
		}
	}

	return nil
}

func listTargets(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()

	all := target.All()
	detected := target.Detected()

	detectedSet := make(map[string]bool, len(detected))
	for _, t := range detected {
		detectedSet[t.Name()] = true
	}

	if outputFormat == "json" {
		type targetJSON struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
			Detected    bool   `json:"detected"`
		}

		items := make([]targetJSON, len(all))
		for i, t := range all {
			items[i] = targetJSON{
				Name:        t.Name(),
				DisplayName: t.DisplayName(),
				Detected:    detectedSet[t.Name()],
			}
		}

		return jsonOutput(out, items)
	}

	for _, t := range all {
		padded := cli.Pad(t.DisplayName(), 18)
		mark := cli.Mute(out, "○")
		name := cli.Mute(out, padded)
		if detectedSet[t.Name()] {
			mark = cli.OK(out, "●")
			name = padded
		}
		cli.Printf(out, "  %s %s %s\n", mark, name, cli.Mute(out, t.Name()))
	}

	return nil
}

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().
		BoolVar(&listOutdatedFlag, "outdated", false, "check installed plugins for available updates")
	listCmd.Flags().
		BoolVar(&listTargetsFlag, "targets", false, "show registered agent targets and detection status")
}
