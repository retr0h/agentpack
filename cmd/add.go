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
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/retr0h/agentpack/internal/cli"
	"github.com/retr0h/agentpack/internal/lock"
	"github.com/retr0h/agentpack/internal/packages"
	"github.com/retr0h/agentpack/internal/safety"
	"github.com/retr0h/agentpack/pkg/install"
	"github.com/retr0h/agentpack/pkg/target"
)

type installer interface {
	Run(ctx context.Context, opts install.Options) (*install.Result, error)
}

var pkgInstaller installer = install.New()

var (
	installSkills  []string
	installTargets []string
	installTrust   bool
	installGlobal  bool
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

		source, atSkill := parseAtSkill(args[0])
		skills := installSkills
		if atSkill != "" {
			skills = append(skills, atSkill)
		}

		displayName := cli.SourceBaseName(source)

		var onStep func(install.Step)
		if outputFormat != "json" {
			cli.Header(out, "adding", displayName)
			onStep = func(s install.Step) {
				cli.StepLine(out, s.Name, s.Detail)
			}
		}

		targets, targetErr := resolveTargets(installTargets)
		if targetErr != nil {
			return targetErr
		}

		result, err := pkgInstaller.Run(ctx, install.Options{
			Source:       source,
			Skills:       skills,
			Targets:      targets,
			Global:       installGlobal,
			OnStep:       onStep,
			ContentCheck: buildContentCheck(cmd, installTrust),
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
		answer := strings.TrimSpace(scanner.Text())

		if answer == "2" {
			return fmt.Errorf("install cancelled by user")
		}

		return nil
	}
}

// parseAtSkill splits "owner/repo@skill" into ("owner/repo", "skill").
// If no @ is present, returns (source, "").
func parseAtSkill(source string) (string, string) {
	if idx := strings.LastIndex(source, "@"); idx > 0 {
		before := source[:idx]
		after := source[idx+1:]
		if after != "" && !strings.Contains(after, "/") {
			return before, after
		}
	}

	return source, ""
}

func resolveTargets(names []string) ([]target.Target, error) {
	if len(names) == 0 {
		return nil, nil
	}

	all := target.All()
	byName := make(map[string]target.Target, len(all))
	for _, t := range all {
		byName[t.Name()] = t
	}

	resolved := make([]target.Target, 0, len(names))
	for _, name := range names {
		t, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("unknown target %q (see agentpack list --targets)", name)
		}

		resolved = append(resolved, t)
	}

	return resolved, nil
}

func init() {
	rootCmd.AddCommand(addCmd)

	addCmd.Flags().
		StringArrayVar(&installSkills, "skill", nil, "include only named skill subdirs from the source (may be repeated)")
	addCmd.Flags().
		StringArrayVar(&installTargets, "target", nil, "install to specific target(s) only (see list --targets)")
	addCmd.Flags().
		BoolVar(&installTrust, "trust", false, "skip executable content prompt (for CI)")
	addCmd.Flags().
		BoolVarP(&installGlobal, "global", "g", false, "install into each agent's global skills directory")
}
