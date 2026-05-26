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
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/retr0h/agentpack/internal/cli"
	"github.com/retr0h/agentpack/internal/lock"
	"github.com/retr0h/agentpack/internal/packages"
	"github.com/retr0h/agentpack/pkg/install"
)

type installer interface {
	Run(ctx context.Context, opts install.Options) (*install.Result, error)
}

var pkgInstaller installer = install.New()

var (
	installSkills []string
	installAgents []string
)

var addCmd = &cobra.Command{
	Use:   "add <source>",
	Short: "Add a plugin from a git repo, archive, or URL",
	Long: `Add a plugin into all detected AI coding agents.
Source may be a git repo, local .agentpack file, or HTTP/HTTPS URL.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		out := cmd.OutOrStdout()

		source := args[0]
		displayName := cli.SourceBaseName(source)

		var onStep func(install.Step)
		if outputFormat != "json" {
			cli.Header(out, "adding", displayName)
			onStep = func(s install.Step) {
				cli.StepLine(out, s.Name, s.Detail)
			}
		}

		result, err := pkgInstaller.Run(ctx, install.Options{
			Source: source,
			Skills: installSkills,
			Agents: installAgents,
			OnStep: onStep,
		})
		if err != nil {
			return err
		}

		if updateErr := updateManifests(source, result); updateErr != nil {
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

// gitHosts lists the domain fragments that indicate a git-hosted source.
var gitHosts = []string{"github.com", "gitlab.com", "bitbucket.org"}

// updateManifests writes the installed package into agentpack-packages.yaml
// and agentpack.lock in the current working directory.
func updateManifests(source string, result *install.Result) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get cwd: %w", err)
	}

	// Update agentpack-packages.yaml.
	pkgPath := filepath.Join(cwd, "agentpack-packages.yaml")

	cfg, err := packages.Load(pkgPath)
	if err != nil {
		return fmt.Errorf("load packages: %w", err)
	}

	pkg := buildPackage(result.Name, source)
	cfg.Add(pkg)

	if err := packages.Save(pkgPath, cfg); err != nil {
		return fmt.Errorf("save packages: %w", err)
	}

	// Update agentpack.lock.
	lockPath := filepath.Join(cwd, "agentpack.lock")

	lf, err := lock.Load(lockPath)
	if err != nil {
		return fmt.Errorf("load lock: %w", err)
	}

	// Strip the #ref fragment for the source field in the lock.
	lockSource := source
	if idx := strings.LastIndex(lockSource, "#"); idx >= 0 {
		lockSource = lockSource[:idx]
	}

	lp := lock.LockedPackage{
		Name:     result.Name,
		Source:   lockSource,
		SHA:      result.SHA,
		Resolved: time.Now().UTC().Format(time.RFC3339),
	}

	if pkg.Ref != "" {
		lp.Ref = pkg.Ref
	}

	lf.Set(lp)

	if err := lock.Save(lockPath, lf); err != nil {
		return fmt.Errorf("save lock: %w", err)
	}

	return nil
}

// buildPackage constructs a packages.Package from the install source URL.
// Git-hosted sources populate the Git (and optionally Ref) fields; everything
// else populates the Source field.
func buildPackage(name, source string) packages.Package {
	pkg := packages.Package{Name: name}

	isGit := false

	for _, host := range gitHosts {
		if strings.Contains(source, host) {
			isGit = true

			break
		}
	}

	if isGit {
		gitURL := source
		ref := ""

		if idx := strings.LastIndex(gitURL, "#"); idx >= 0 {
			ref = gitURL[idx+1:]
			gitURL = gitURL[:idx]
		}

		pkg.Git = gitURL
		pkg.Ref = ref
	} else {
		pkg.Source = source
	}

	return pkg
}

func init() {
	rootCmd.AddCommand(addCmd)

	addCmd.Flags().
		StringArrayVar(&installSkills, "skill", nil, "add only specific skill(s) by name (may be repeated)")
	addCmd.Flags().
		StringArrayVar(&installAgents, "agent", nil, "add only specific agent(s) by name (may be repeated)")
}
