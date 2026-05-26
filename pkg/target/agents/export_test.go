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

package agents

import "os"

// NewAgentWithFuncs exposes newAgent for testing with injectable home and cwd
// functions.
func NewAgentWithFuncs(
	def AgentDef,
	homeFunc func() (string, error),
	cwdFunc func() (string, error),
) *agent {
	return newAgent(def, homeFunc, cwdFunc)
}

// NewAgentWithGetenv returns an agent with an injectable getenv function so
// tests can exercise EnvOverride without t.Setenv (which is incompatible with
// t.Parallel).
func NewAgentWithGetenv(
	def AgentDef,
	homeFunc func() (string, error),
	getenvFunc func(string) string,
) *agent {
	a := newAgent(def, homeFunc, os.Getwd)
	a.getenvFunc = getenvFunc
	return a
}

// Registry exposes the package-level registry slice for inspection in tests.
var Registry = registry
