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

package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/retr0h/claudia/pkg/cli"
)

func TestBanner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantTop   string
		wantBot   string
		wantLines int
	}{
		{
			name:      "contains both banner lines",
			wantTop:   "█▀▀ █░░ █▀█ █░█ █▀▄ █ █▀█",
			wantBot:   "█▄▄ █▄▄ █▀█ █▄█ █▄▀ █ █▀█",
			wantLines: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			got := cli.Banner(&buf)
			lines := strings.Split(strings.TrimRight(got, "\n"), "\n")

			if len(lines) != tt.wantLines {
				t.Fatalf("line count = %d, want %d", len(lines), tt.wantLines)
			}
			if !strings.Contains(got, tt.wantTop) {
				t.Errorf("missing top line %q in %q", tt.wantTop, got)
			}
			if !strings.Contains(got, tt.wantBot) {
				t.Errorf("missing bot line %q in %q", tt.wantBot, got)
			}
		})
	}
}

func TestThemeRenders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		render func(w *bytes.Buffer) string
		want   string
	}{
		{
			name:   "Mute contains input text",
			render: func(w *bytes.Buffer) string { return cli.Mute(w, "dim") },
			want:   "dim",
		},
		{
			name:   "Accent contains input text",
			render: func(w *bytes.Buffer) string { return cli.Accent(w, "violet") },
			want:   "violet",
		},
		{
			name:   "OK contains input text",
			render: func(w *bytes.Buffer) string { return cli.OK(w, "pass") },
			want:   "pass",
		},
		{
			name:   "Err contains input text",
			render: func(w *bytes.Buffer) string { return cli.Err(w, "fail") },
			want:   "fail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			got := tt.render(&buf)
			if !strings.Contains(got, tt.want) {
				t.Errorf("render = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}
