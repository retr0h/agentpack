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

// Package cmd contains the agentpack cobra command tree.
package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/retr0h/agentpack/internal/cli"
	_ "github.com/retr0h/agentpack/pkg/target/agents"     // register data-driven agents (includes universal and windsurf)
	_ "github.com/retr0h/agentpack/pkg/target/claudecode" // register Claude Code target
	_ "github.com/retr0h/agentpack/pkg/target/cursor"     // register Cursor target
)

var outputFormat string

var rootCmd = &cobra.Command{
	Use:     "agentpack",
	Short:   "The native package manager for agentskills.io",
	Version: version,
}

// Execute runs the root command; invoked by main.
func Execute() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true

	rootCmd.PersistentFlags().
		StringVarP(&outputFormat, "output", "o", "text", "output format (text, json)")

	defaultHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		if c == rootCmd {
			out := c.OutOrStdout()
			cli.Print(out, "")
			cli.Print(out, cli.Banner(out))
		}
		defaultHelp(c, args)
	})

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		out := rootCmd.ErrOrStderr()
		cli.Printf(out, "  %s %s\n\n", cli.Err(out, "✗"), err.Error())
		os.Exit(1)
	}
}
