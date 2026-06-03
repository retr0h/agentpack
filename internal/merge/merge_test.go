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

package merge_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/retr0h/agentpack/internal/merge"
)

func TestStrings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    []string
		b    []string
		want []string
	}{
		{
			name: "both nil returns nil",
			a:    nil,
			b:    nil,
			want: nil,
		},
		{
			name: "both empty returns nil",
			a:    []string{},
			b:    []string{},
			want: nil,
		},
		{
			name: "a only returns sorted a",
			a:    []string{"c", "a"},
			b:    nil,
			want: []string{"a", "c"},
		},
		{
			name: "b only returns sorted b",
			a:    nil,
			b:    []string{"z", "m"},
			want: []string{"m", "z"},
		},
		{
			name: "deduplicates overlapping entries",
			a:    []string{"x", "y"},
			b:    []string{"y", "z"},
			want: []string{"x", "y", "z"},
		},
		{
			name: "no overlap appends and sorts",
			a:    []string{"b"},
			b:    []string{"a"},
			want: []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := merge.Strings(tt.a, tt.b)
			assert.Equal(t, tt.want, got)
		})
	}
}
