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

package drivers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/pkg/drivers"
)

func TestList(t *testing.T) {
	drivers.RegisterAll()

	infos := drivers.List()

	byName := make(map[string]drivers.Info, len(infos))
	for _, info := range infos {
		byName[info.Name] = info
	}

	tests := []struct {
		name            string
		driver          string
		wantDisplayName string
	}{
		{
			name:            "claude-code is registered",
			driver:          "claude-code",
			wantDisplayName: "Claude Code",
		},
		{
			name:            "cursor is registered",
			driver:          "cursor",
			wantDisplayName: "Cursor",
		},
		{
			name:            "codex is registered",
			driver:          "codex",
			wantDisplayName: "Codex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, ok := byName[tt.driver]
			require.True(t, ok)
			assert.Equal(t, tt.wantDisplayName, info.DisplayName)
		})
	}
}
