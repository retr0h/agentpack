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

	"github.com/spf13/cobra"

	"github.com/retr0h/agentpack/pkg/cli"
	"github.com/retr0h/agentpack/pkg/verify"
)

var verifyCmd = &cobra.Command{
	Use:   "verify <archive.agentpack>",
	Short: "Verify checksums of a .agentpack archive",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		out := cmd.OutOrStdout()

		result, err := verify.Run(ctx, args[0])
		if err != nil {
			return err
		}

		cli.Printf(
			out, "%s %s\n\n",
			cli.Mute(out, "agentpack: verifying"),
			cli.Accent(out, result.ArchiveName),
		)

		passed := 0
		failed := 0

		for _, f := range result.Files {
			if f.OK {
				cli.Printf(out, "  %-60s %s\n", f.Path, cli.OK(out, "OK"))
				passed++
			} else {
				cli.Printf(out, "  %-60s %s  %s\n", f.Path, cli.Err(out, "FAIL"), f.Err)
				failed++
			}
		}

		total := passed + failed
		cli.Printf(out, "\n  %d/%d files verified\n", passed, total)

		if failed > 0 {
			return fmt.Errorf("%d file(s) failed verification", failed)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}
