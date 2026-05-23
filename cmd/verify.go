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

	"github.com/retr0h/claudia/pkg/verify"
)

var verifyCmd = &cobra.Command{
	Use:   "verify <archive.claudia>",
	Short: "Verify checksums of a .claudia archive",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		result, err := verify.Run(ctx, args[0])
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "claudia: verifying %s\n\n", result.ArchiveName)

		passed := 0
		failed := 0

		for _, f := range result.Files {
			if f.OK {
				fmt.Fprintf(cmd.OutOrStdout(), "  %-60s OK\n", f.Path)
				passed++
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "  %-60s FAIL  %s\n", f.Path, f.Err)
				failed++
			}
		}

		total := passed + failed
		fmt.Fprintf(cmd.OutOrStdout(), "\n  %d/%d files verified\n", passed, total)

		if failed > 0 {
			return fmt.Errorf("%d file(s) failed verification", failed)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}
