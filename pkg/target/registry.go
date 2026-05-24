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

package target

// registry holds all known targets, populated via init() in each driver
// package.
var registry []Target //nolint:gochecknoglobals

// Register adds a target to the global registry. It is called from each
// driver package's init() function so that a blank import is sufficient to
// activate the driver.
func Register(t Target) {
	registry = append(registry, t)
}

// All returns a copy of all registered targets in registration order.
func All() []Target {
	out := make([]Target, len(registry))
	copy(out, registry)

	return out
}

// Detected returns the subset of registered targets whose Detect() method
// returns true — i.e. the agents that are installed on the current system.
func Detected() []Target {
	var out []Target

	for _, t := range registry {
		if t.Detect() {
			out = append(out, t)
		}
	}

	return out
}
