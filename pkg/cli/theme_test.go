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
	"os"
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
		{
			name:   "Info contains input text",
			render: func(w *bytes.Buffer) string { return cli.Info(w, "2026-05-23") },
			want:   "2026-05-23",
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

func TestPrintf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		format string
		args   []any
		want   string
	}{
		{
			name:   "writes formatted string",
			format: "hello %s %d",
			args:   []any{"world", 42},
			want:   "hello world 42",
		},
		{
			name:   "handles empty format",
			format: "",
			args:   nil,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			cli.Printf(&buf, tt.format, tt.args...)
			if buf.String() != tt.want {
				t.Errorf("Printf output = %q, want %q", buf.String(), tt.want)
			}
		})
	}
}

func TestPrint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "writes line with newline",
			text: "hello",
			want: "hello\n",
		},
		{
			name: "empty string produces newline",
			text: "",
			want: "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			cli.Print(&buf, tt.text)
			if buf.String() != tt.want {
				t.Errorf("Print output = %q, want %q", buf.String(), tt.want)
			}
		})
	}
}

func TestPad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    string
		w    int
		want string
	}{
		{
			name: "pads short string to width",
			s:    "hi",
			w:    6,
			want: "hi    ",
		},
		{
			name: "returns unchanged when equal width",
			s:    "hello",
			w:    5,
			want: "hello",
		},
		{
			name: "returns unchanged when longer than width",
			s:    "toolong",
			w:    3,
			want: "toolong",
		},
		{
			name: "pads empty string",
			s:    "",
			w:    4,
			want: "    ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := cli.Pad(tt.s, tt.w)
			if got != tt.want {
				t.Errorf("Pad(%q, %d) = %q, want %q", tt.s, tt.w, got, tt.want)
			}
		})
	}
}

func TestHumanSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{
			name:  "zero bytes",
			bytes: 0,
			want:  "0 B",
		},
		{
			name:  "under 1 KB",
			bytes: 512,
			want:  "512 B",
		},
		{
			name:  "exactly 1 KB",
			bytes: 1024,
			want:  "1 KB",
		},
		{
			name:  "multiple KB",
			bytes: 5120,
			want:  "5 KB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := cli.HumanSize(tt.bytes)
			if got != tt.want {
				t.Errorf("HumanSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestBannerWithFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantTop string
		wantBot string
	}{
		{
			name:    "renders banner to os.File writer",
			wantTop: "█▀▀ █░░ █▀█ █░█ █▀▄ █ █▀█",
			wantBot: "█▄▄ █▄▄ █▀█ █▄█ █▄▀ █ █▀█",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Use a real *os.File to exercise the rendererFor *os.File branch.
			f, err := os.CreateTemp(t.TempDir(), "banner-*.txt")
			if err != nil {
				t.Fatalf("create temp file: %v", err)
			}
			defer func() { _ = f.Close() }()

			got := cli.Banner(f)
			if !strings.Contains(got, tt.wantTop) {
				t.Errorf("missing top line %q in %q", tt.wantTop, got)
			}
			if !strings.Contains(got, tt.wantBot) {
				t.Errorf("missing bot line %q in %q", tt.wantBot, got)
			}
		})
	}
}
