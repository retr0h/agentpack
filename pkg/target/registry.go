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

import "fmt"

// registry holds all known targets, populated by driver.RegisterAll().
var registry []Target //nolint:gochecknoglobals

// Register adds a target to the global registry. It is called from
// driver.RegisterAll() during application startup.
func Register(t Target) {
	registry = append(registry, t)
}

// All returns a copy of all registered targets in registration order.
func All() []Target {
	out := make([]Target, len(registry))
	copy(out, registry)

	return out
}

// Resolve maps a list of target names to their registered Target values.
// It returns an error when any name is not found in the registry.
// When names is empty, nil is returned.
func Resolve(names []string) ([]Target, error) {
	if len(names) == 0 {
		return nil, nil
	}

	all := All()
	byName := make(map[string]Target, len(all))
	for _, t := range all {
		byName[t.Name()] = t
	}

	resolved := make([]Target, 0, len(names))
	for _, name := range names {
		t, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("unknown target %q (see agentpack list --targets)", name)
		}

		resolved = append(resolved, t)
	}

	return resolved, nil
}

// Detected returns the subset of registered targets whose Detect() method
// returns true — i.e. the agents that are installed on the current system.
func Detected() []Target {
	var out []Target

	for _, t := range All() {
		if t.Detect() {
			out = append(out, t)
		}
	}

	return out
}
