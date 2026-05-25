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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/retr0h/agentpack/pkg/cli"
	"github.com/retr0h/agentpack/pkg/verify"
)

var verifySHA256 string

var verifyCmd = &cobra.Command{
	Use:   "verify <archive.agentpack>",
	Short: "Verify checksums of a .agentpack archive",
	Long: `Verify internal checksums of a .agentpack archive.

With --sha256, also verify the archive itself against an external hash.
This provides tamper detection for distributed archives — the builder
publishes the SHA256 alongside the archive (like goreleaser checksums.txt).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		out := cmd.OutOrStdout()

		archivePath := args[0]

		// Auto-detect .sha256 file alongside the archive if --sha256 not given.
		if verifySHA256 == "" {
			shaFile := strings.TrimSuffix(archivePath, ".agentpack") + ".sha256"
			if data, err := os.ReadFile(shaFile); err == nil {
				verifySHA256 = strings.TrimSpace(string(data))
			}
		}

		// External SHA256 verification (tamper detection).
		if verifySHA256 != "" {
			data, err := os.ReadFile(archivePath)
			if err != nil {
				return fmt.Errorf("read archive: %w", err)
			}

			h := sha256.Sum256(data)
			actual := hex.EncodeToString(h[:])

			if actual != verifySHA256 {
				return fmt.Errorf(
					"archive SHA256 mismatch\n  expected: %s\n  actual:   %s",
					verifySHA256, actual,
				)
			}

			cli.Printf(out, "  %s %s\n", cli.OK(out, checkmark), cli.Mute(out, "archive SHA256 verified"))
		}

		// Internal checksum verification (corruption detection).
		result, err := verify.Run(ctx, archivePath)
		if err != nil {
			return err
		}

		cli.Printf(out, "%s %s\n\n",
			cli.Mute(out, "agentpack: verifying"),
			cli.Accent(out, result.ArchiveName),
		)

		passed := 0
		failed := 0

		for _, f := range result.Files {
			if f.OK {
				passed++
			} else {
				cli.Printf(out, "  %-60s %s  %s\n", f.Path, cli.Err(out, "FAIL"), f.Err)
				failed++
			}
		}

		total := passed + failed
		cli.Printf(out, "  %s %s\n",
			cli.OK(out, checkmark),
			cli.Mute(out, fmt.Sprintf("internal checksums %d/%d OK", passed, total)),
		)

		if failed > 0 {
			return fmt.Errorf("%d file(s) failed verification", failed)
		}

		return nil
	},
}

func init() {
	verifyCmd.Flags().StringVar(&verifySHA256, "sha256", "", "verify archive against external SHA256 hash")
	rootCmd.AddCommand(verifyCmd)
}
