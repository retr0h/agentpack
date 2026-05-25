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

// Package update re-installs an agentpack plugin from its stored source.
package update

import (
	"context"
	"fmt"
	"strings"

	"github.com/retr0h/agentpack/pkg/fetcher"
	"github.com/retr0h/agentpack/pkg/install"
	"github.com/retr0h/agentpack/pkg/registry"
)

// Options configures an update run.
type Options struct {
	Name   string
	OnStep func(install.Step)
}

// Result holds the outcome of an update.
type Result struct {
	Name    string
	OldSHA  string
	NewSHA  string
	Version string
	Updated bool
}

func shortSHA(s string) string {
	if len(s) >= 7 {
		return s[:7]
	}

	return s
}

// Run checks if an update is available and re-installs only if the remote
// SHA differs from the installed SHA.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m, err := registry.Load(opts.Name)
	if err != nil {
		return nil, fmt.Errorf("load registry manifest: %w", err)
	}

	oldSHA := shortSHA(m.SHA)

	// Check if the source is a git repo — if so, check remote HEAD first.
	f, fErr := fetcher.New(m.Source)
	if fErr == nil {
		if _, isGit := f.(*fetcher.GitFetcher); isGit {
			if opts.OnStep != nil {
				opts.OnStep(install.Step{Name: "checking", Detail: m.Source})
			}

			refs, lsErr := fetcher.LsRemote(ctx, m.Source)
			if lsErr == nil {
				remoteSHA := resolveHEAD(refs)
				remoteShort := shortSHA(remoteSHA)
				if remoteSHA != "" && (remoteShort == oldSHA || strings.HasPrefix(remoteSHA, m.SHA)) {
					return &Result{
						Name:    opts.Name,
						OldSHA:  oldSHA,
						NewSHA:  oldSHA,
						Version: m.Version,
						Updated: false,
					}, nil
				}
			}
		}
	}

	installResult, err := install.Run(ctx, install.Options{
		Source: m.Source,
		OnStep: opts.OnStep,
	})
	if err != nil {
		return nil, fmt.Errorf("re-install: %w", err)
	}

	return &Result{
		Name:    installResult.Name,
		OldSHA:  oldSHA,
		NewSHA:  installResult.SHA,
		Version: installResult.Version,
		Updated: oldSHA != installResult.SHA,
	}, nil
}

// resolveHEAD finds the actual HEAD SHA from ls-remote refs. go-git returns
// HEAD as a zero hash for symbolic refs, so we fall back to refs/heads/main
// or refs/heads/master.
func resolveHEAD(refs map[string]string) string {
	if sha, ok := refs["HEAD"]; ok && sha != "0000000000000000000000000000000000000000" {
		return sha
	}

	if sha, ok := refs["refs/heads/main"]; ok {
		return sha
	}

	if sha, ok := refs["refs/heads/master"]; ok {
		return sha
	}

	return ""
}
