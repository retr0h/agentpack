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

// Package install export_test.go exposes private helpers for white-box testing.
package install

import (
	"context"
	"errors"
	"os"

	"github.com/retr0h/agentpack/pkg/metadata"
)

// ShortSHA exposes shortSHA for testing.
func ShortSHA(sha string) string {
	return shortSHA(sha)
}

// CopyDir exposes copyDir for testing.
func CopyDir(ctx context.Context, src string, dst string) error {
	return copyDir(ctx, src, dst)
}

// CopyFile exposes copyFile for testing.
func CopyFile(src string, dst string) error {
	return copyFile(src, dst)
}

// FindChecksums exposes findChecksums for testing.
func FindChecksums(dir string) (string, error) {
	return findChecksums(dir)
}

// FindAndReadMetadata exposes findAndReadMetadata for testing.
func FindAndReadMetadata(dir string) (*metadata.Metadata, error) {
	return findAndReadMetadata(dir)
}

// SetOsCreateTemp replaces osCreateTemp for testing.
func SetOsCreateTemp(fn func(string, string) (*os.File, error)) func() {
	orig := osCreateTemp
	osCreateTemp = fn

	return func() { osCreateTemp = orig }
}

// SetOsMkdirTemp replaces osMkdirTemp for testing.
func SetOsMkdirTemp(fn func(string, string) (string, error)) func() {
	orig := osMkdirTemp
	osMkdirTemp = fn

	return func() { osMkdirTemp = orig }
}

// CreateTempAlwaysFails is an osCreateTemp that always returns an error.
func CreateTempAlwaysFails(_, _ string) (*os.File, error) {
	return nil, errors.New("simulated create temp failure")
}

// MkdirTempAlwaysFails is an osMkdirTemp that always returns an error.
func MkdirTempAlwaysFails(_, _ string) (string, error) {
	return "", errors.New("simulated mkdir temp failure")
}
