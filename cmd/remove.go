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
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/retr0h/agentpack/internal/cli"
	"github.com/retr0h/agentpack/internal/configmerge"
	"github.com/retr0h/agentpack/internal/lock"
	"github.com/retr0h/agentpack/internal/packages"
	pkgremove "github.com/retr0h/agentpack/pkg/remove"
)

type remover interface {
	Run(ctx context.Context, opts pkgremove.Options) (*pkgremove.Result, error)
}

var pkgRemover remover = pkgremove.New()

var delGlobal bool

var delCmd = &cobra.Command{
	Use:   "del <name[@skill]>",
	Short: "Delete an installed agentpack plugin",
	Long: `Delete an installed agentpack plugin. Only the exact files recorded in
the plugin registry are deleted. User-modified files are skipped. The .git
directory is never touched.

To remove a single skill from a package without deleting the entire package,
append @skill to the package name: agentpack del my-package@my-skill`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		out := cmd.OutOrStdout()

		name, skill := parseAtSkill(args[0])

		displayName := name
		if skill != "" {
			displayName = name + "@" + skill
		}

		var onStep func(pkgremove.Step)
		if outputFormat != "json" {
			cli.Header(out, "deleting", displayName)
			onStep = func(s pkgremove.Step) {
				if s.Skipped {
					cli.Printf(out, "  %s %s\n", cli.Mute(out, "skipped"), cli.Mute(out, s.Path))
				} else {
					cli.StepLine(out, "removed", s.Path)
				}
			}
		}

		result, err := pkgRemover.Run(ctx, pkgremove.Options{
			Name:   name,
			Skill:  skill,
			Global: delGlobal,
			OnStep: onStep,
		})
		if err != nil {
			return err
		}

		removeManifests(name, skill)

		if outputFormat == "json" {
			return jsonOutput(out, result)
		}

		cli.Printf(
			out,
			"\n  %s %s %s\n",
			cli.OK(out, cli.Checkmark),
			cli.Accent(out, result.Name),
			cli.Mute(out, "deleted"),
		)

		return nil
	},
}

// removeManifests removes the named package from agentpack-packages.yaml,
// agentpack.lock, and the hooks section of .claude/settings.json. All
// operations are best-effort: a missing file or missing entry is not an error,
// because users may have installed a package without a managed yaml/lock.
func removeManifests(name, skill string) {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}

	pkgPath := filepath.Join(cwd, "agentpack-packages.yaml")
	lockPath := filepath.Join(cwd, "agentpack.lock")

	if skill != "" {
		if cfg, loadErr := packages.Load(pkgPath); loadErr == nil {
			if p := cfg.Find(name); p != nil {
				remaining := make([]string, 0, len(p.Skills))
				for _, s := range p.Skills {
					if s != skill {
						remaining = append(remaining, s)
					}
				}

				p.Skills = remaining
			}

			_ = packages.Save(pkgPath, cfg)
		}

		return
	}

	if cfg, loadErr := packages.Load(pkgPath); loadErr == nil {
		cfg.Remove(name)
		_ = packages.Save(pkgPath, cfg)
	}

	if lf, loadErr := lock.Load(lockPath); loadErr == nil {
		lf.Remove(name)
		_ = lock.Save(lockPath, lf)
	}

	settingsPath := filepath.Join(cwd, ".claude", "settings.json")
	_ = configmerge.RemoveHooks(settingsPath, name)
}

func init() {
	rootCmd.AddCommand(delCmd)

	delCmd.Flags().
		BoolVarP(&delGlobal, "global", "g", false, "remove a globally installed plugin")
}
