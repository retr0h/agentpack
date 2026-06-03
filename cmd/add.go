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
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/retr0h/agentpack/internal/cli"
	"github.com/retr0h/agentpack/internal/safety"
	"github.com/retr0h/agentpack/pkg/install"
	"github.com/retr0h/agentpack/pkg/target"
)

type installer interface {
	Run(ctx context.Context, opts install.Options) (*install.Result, error)
}

var pkgInstaller installer = install.New()

var (
	installTargets []string
	installTrust   bool
	installGlobal  bool
)

var addCmd = &cobra.Command{
	Use:   "add <source[@ref][:type/name]...>",
	Short: "Add a plugin from a git repo, archive, or URL",
	Long: `Add a plugin into all detected AI coding agents.
Source may be a git repo, local .agentpack file, or HTTP/HTTPS URL.

Use @ to pin a version: agentpack add owner/repo@v2.0.0
Use : to select content:  agentpack add owner/repo:skill/k8s:command/scan`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		out := cmd.OutOrStdout()

		spec := install.ParseSource(args[0])

		displayName := cli.SourceBaseName(spec.Source)

		var onStep func(install.Step)
		if outputFormat != "json" {
			cli.Header(out, "adding", displayName)
			onStep = func(s install.Step) {
				cli.StepLine(out, s.Name, s.Detail)
			}
		}

		targets, targetErr := target.Resolve(installTargets)
		if targetErr != nil {
			return targetErr
		}

		result, err := pkgInstaller.Run(ctx, install.Options{
			Source:       spec.Source,
			Ref:          spec.Ref,
			Selectors:    spec.Selectors,
			Targets:      targets,
			Global:       installGlobal,
			OnStep:       onStep,
			ContentCheck: buildContentCheck(cmd, installTrust),
		})
		if err != nil {
			return err
		}

		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return fmt.Errorf("get cwd: %w", cwdErr)
		}

		content := install.SelectorsToContent(spec.Selectors)
		if updateErr := install.UpdateManifests(cwd, spec.Source, content, installTargets, spec.Ref, result); updateErr != nil {
			return updateErr
		}

		if outputFormat == "json" {
			return jsonOutput(out, result)
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
			rows = append(
				rows,
				targetRow{displayName: dn, targetName: tn, count: result.FileCounts[dn]},
			)
		}

		sort.Slice(rows, func(i, j int) bool {
			return rows[i].displayName < rows[j].displayName
		})

		for i, row := range rows {
			prefix := "  ├─"
			if i == len(rows)-1 {
				prefix = "  └─"
			}
			cli.Printf(
				out,
				"%s %s  %s\n",
				cli.Mute(out, prefix),
				cli.Tag(out, cli.Pad(row.targetName, 12)),
				cli.Mute(
					out,
					fmt.Sprintf("(%d %s)", row.count, cli.Plural(row.count, "file", "files")),
				),
			)
		}

		cli.Printf(
			out,
			"\n  %s %s %s\n",
			cli.OK(out, cli.Checkmark),
			cli.Accent(out, result.Name),
			cli.Mute(out, "installed"),
		)

		return nil
	},
}

// buildContentCheck returns a ContentCheck callback for install.Options.
// When trust is true or the terminal is not a TTY, it returns nil (no check).
// Otherwise it prompts the user for confirmation when executable files exist.
func buildContentCheck(cmd *cobra.Command, trust bool) func(*safety.Classification) error {
	if trust {
		return nil
	}

	out := cmd.OutOrStdout()

	return func(c *safety.Classification) error {
		if len(c.Executable) == 0 {
			return nil
		}

		if f, ok := out.(*os.File); !ok || !isatty.IsTerminal(f.Fd()) {
			return nil
		}

		cli.Printf(out, "\n%s Package contains executable content:\n", cli.Err(out, "!"))

		for _, f := range c.Executable {
			cli.Printf(out, "  %s\n", cli.Mute(out, f))
		}

		cli.Printf(
			out,
			"\n%s\n\n> 1. Yes, I trust this package\n  2. No, cancel\n\nChoice [1]: ",
			cli.Mute(out, "Allow? Only add packages from sources you trust."),
		)

		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("read input: %w", err)
		}
		answer := strings.TrimSpace(scanner.Text())

		if answer == "2" {
			return fmt.Errorf("install cancelled by user")
		}

		return nil
	}
}

func init() {
	rootCmd.AddCommand(addCmd)

	addCmd.Flags().
		StringArrayVar(&installTargets, "target", nil, "install to specific target(s) only (see list --targets)")
	addCmd.Flags().
		BoolVar(&installTrust, "trust", false, "skip executable content prompt (for CI)")
	addCmd.Flags().
		BoolVarP(&installGlobal, "global", "g", false, "install into each agent's global skills directory")
}
