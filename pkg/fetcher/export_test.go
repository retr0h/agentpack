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

package fetcher

import (
	"io"
)

// SetOsUserHomeDir replaces the osUserHomeDir function for testing.
func SetOsUserHomeDir(fn func() (string, error)) func() {
	orig := osUserHomeDir
	osUserHomeDir = fn
	return func() { osUserHomeDir = orig }
}

// SetIoCopyFile replaces the ioCopyFile function for testing.
func SetIoCopyFile(fn func(io.Writer, io.Reader) (int64, error)) func() {
	orig := ioCopyFile
	ioCopyFile = fn
	return func() { ioCopyFile = orig }
}

// SetOsCreateFile replaces the osCreateFile function for testing.
func SetOsCreateFile(fn func(string) (io.WriteCloser, error)) func() {
	orig := osCreateFile
	osCreateFile = fn
	return func() { osCreateFile = orig }
}

// SetOsCreateHTTP replaces the osCreateHTTP function for testing.
func SetOsCreateHTTP(fn func(string) (io.WriteCloser, error)) func() {
	orig := osCreateHTTP
	osCreateHTTP = fn
	return func() { osCreateHTTP = orig }
}

// ParseGitSource exposes parseGitSource for testing.
func ParseGitSource(source string) (rawURL, ref string, err error) {
	return parseGitSource(source)
}

// ToGitURL exposes toGitURL for testing.
func ToGitURL(rawURL string) string {
	return toGitURL(rawURL)
}

// IsSHA exposes isSHA for testing.
func IsSHA(ref string) bool {
	return isSHA(ref)
}
