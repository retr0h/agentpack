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

	"github.com/retr0h/agentpack/internal/cli"
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
			wantTop:   "█▀█ █▀▀ █▀▀ █▄░█ ▀█▀ █▀█ █▀█ █▀▀ █▄▀",
			wantBot:   "█▀█ █▄█ ██▄ █░▀█ ░█░ █▀▀ █▀█ █▄▄ █░█",
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

func TestCheckmark(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{
			name: "is Unicode check mark",
			want: "✓",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if cli.Checkmark != tt.want {
				t.Errorf("Checkmark = %q, want %q", cli.Checkmark, tt.want)
			}
		})
	}
}

func TestPlural(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		n          int
		singular   string
		pluralForm string
		want       string
	}{
		{
			name:       "returns singular when n is 1",
			n:          1,
			singular:   "plugin",
			pluralForm: "plugins",
			want:       "plugin",
		},
		{
			name:       "returns plural when n is 0",
			n:          0,
			singular:   "plugin",
			pluralForm: "plugins",
			want:       "plugins",
		},
		{
			name:       "returns plural when n is 2",
			n:          2,
			singular:   "file",
			pluralForm: "files",
			want:       "files",
		},
		{
			name:       "returns plural when n is large",
			n:          100,
			singular:   "item",
			pluralForm: "items",
			want:       "items",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := cli.Plural(tt.n, tt.singular, tt.pluralForm)
			if got != tt.want {
				t.Errorf("Plural(%d, %q, %q) = %q, want %q", tt.n, tt.singular, tt.pluralForm, got, tt.want)
			}
		})
	}
}

func TestShortSHA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sha  string
		want string
	}{
		{
			name: "truncates long SHA to 7 characters",
			sha:  "abcdef1234567890",
			want: "abcdef1",
		},
		{
			name: "returns unchanged when exactly 7 characters",
			sha:  "abcdef1",
			want: "abcdef1",
		},
		{
			name: "returns unchanged when shorter than 7",
			sha:  "abc",
			want: "abc",
		},
		{
			name: "returns empty string for empty input",
			sha:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := cli.ShortSHA(tt.sha)
			if got != tt.want {
				t.Errorf("ShortSHA(%q) = %q, want %q", tt.sha, got, tt.want)
			}
		})
	}
}

func TestSourceBaseName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "extracts last path segment from URL",
			source: "https://github.com/org/my-plugin",
			want:   "my-plugin",
		},
		{
			name:   "strips .git suffix",
			source: "https://github.com/org/my-plugin.git",
			want:   "my-plugin",
		},
		{
			name:   "strips ref fragment",
			source: "https://github.com/org/my-plugin#main",
			want:   "my-plugin",
		},
		{
			name:   "strips trailing slash",
			source: "https://github.com/org/my-plugin/",
			want:   "my-plugin",
		},
		{
			name:   "strips .git and fragment together",
			source: "https://github.com/org/my-plugin.git#abc123",
			want:   "my-plugin",
		},
		{
			name:   "returns plain name when no slashes",
			source: "my-plugin",
			want:   "my-plugin",
		},
		{
			name:   "handles local file path",
			source: "/home/user/archives/my-plugin.agentpack",
			want:   "my-plugin.agentpack",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := cli.SourceBaseName(tt.source)
			if got != tt.want {
				t.Errorf("SourceBaseName(%q) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}

func TestStepLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stepName   string
		detail     string
		wantSubstr string
	}{
		{
			name:       "contains checkmark name and detail",
			stepName:   "downloading",
			detail:     "v1.2.3",
			wantSubstr: "downloading",
		},
		{
			name:       "contains checkmark character",
			stepName:   "step",
			detail:     "",
			wantSubstr: "✓",
		},
		{
			name:       "ends with newline",
			stepName:   "done",
			detail:     "ok",
			wantSubstr: "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			cli.StepLine(&buf, tt.stepName, tt.detail)
			got := buf.String()
			if !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("StepLine output %q does not contain %q", got, tt.wantSubstr)
			}
		})
	}
}

func TestTreeRow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		isLast     bool
		rowName    string
		nameWidth  int
		detail     string
		wantSubstr string
	}{
		{
			name:       "non-last row uses branch connector",
			isLast:     false,
			rowName:    "claude",
			nameWidth:  12,
			detail:     "(3 files)",
			wantSubstr: "├─",
		},
		{
			name:       "last row uses end connector",
			isLast:     true,
			rowName:    "cursor",
			nameWidth:  12,
			detail:     "(1 file)",
			wantSubstr: "└─",
		},
		{
			name:       "contains row name",
			isLast:     false,
			rowName:    "universal",
			nameWidth:  12,
			detail:     "(5 files)",
			wantSubstr: "universal",
		},
		{
			name:       "contains detail text",
			isLast:     true,
			rowName:    "target",
			nameWidth:  12,
			detail:     "(2 files)",
			wantSubstr: "(2 files)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			cli.TreeRow(&buf, tt.isLast, tt.rowName, tt.nameWidth, tt.detail)
			got := buf.String()
			if !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("TreeRow output %q does not contain %q", got, tt.wantSubstr)
			}
		})
	}
}

func TestHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		action     string
		pluginName string
		wantSubstr string
	}{
		{
			name:       "contains action text",
			action:     "installing",
			pluginName: "my-plugin",
			wantSubstr: "installing",
		},
		{
			name:       "contains plugin name",
			action:     "removing",
			pluginName: "my-plugin",
			wantSubstr: "my-plugin",
		},
		{
			name:       "ends with double newline",
			action:     "updating",
			pluginName: "thing",
			wantSubstr: "\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			cli.Header(&buf, tt.action, tt.pluginName)
			got := buf.String()
			if !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("Header output %q does not contain %q", got, tt.wantSubstr)
			}
		})
	}
}

func TestField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		label      string
		value      string
		wantSubstr string
	}{
		{
			name:       "contains label with colon",
			label:      "Name",
			value:      "my-plugin",
			wantSubstr: "Name:",
		},
		{
			name:       "contains value",
			label:      "Version",
			value:      "1.2.3",
			wantSubstr: "1.2.3",
		},
		{
			name:       "ends with newline",
			label:      "Source",
			value:      "https://example.com",
			wantSubstr: "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			cli.Field(&buf, tt.label, tt.value)
			got := buf.String()
			if !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("Field output %q does not contain %q", got, tt.wantSubstr)
			}
		})
	}
}

func TestFieldMuted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		label      string
		value      string
		wantSubstr string
	}{
		{
			name:       "contains label with colon",
			label:      "SHA256",
			value:      "~/.config/agentpack/archives/x.sha256",
			wantSubstr: "SHA256:",
		},
		{
			name:       "contains value",
			label:      "Archive",
			value:      "~/.config/agentpack/archives/x.agentpack",
			wantSubstr: "~/.config/agentpack/archives/x.agentpack",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			cli.FieldMuted(&buf, tt.label, tt.value)
			got := buf.String()
			if !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("FieldMuted output %q does not contain %q", got, tt.wantSubstr)
			}
		})
	}
}

func TestTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cols       []cli.TableColumn
		wantSubstr string
		wantAbsent string
	}{
		{
			name: "renders headers",
			cols: []cli.TableColumn{
				{Header: "NAME", Values: []string{"alpha"}},
				{Header: "VERSION", Values: []string{"1.0"}},
			},
			wantSubstr: "NAME",
		},
		{
			name: "renders row values",
			cols: []cli.TableColumn{
				{Header: "NAME", Values: []string{"my-plugin"}, Accent: true},
				{Header: "VERSION", Values: []string{"2.3.4"}},
			},
			wantSubstr: "my-plugin",
		},
		{
			name: "renders multiple rows",
			cols: []cli.TableColumn{
				{Header: "NAME", Values: []string{"alpha", "beta"}, Accent: true},
				{Header: "VERSION", Values: []string{"1.0", "2.0"}},
			},
			wantSubstr: "beta",
		},
		{
			name:       "empty columns renders nothing",
			cols:       []cli.TableColumn{},
			wantSubstr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			cli.Table(&buf, tt.cols)
			got := buf.String()
			if tt.wantSubstr != "" && !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("Table output %q does not contain %q", got, tt.wantSubstr)
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
			wantTop: "█▀█ █▀▀ █▀▀ █▄░█ ▀█▀ █▀█ █▀█ █▀▀ █▄▀",
			wantBot: "█▀█ █▄█ ██▄ █░▀█ ░█░ █▀▀ █▀█ █▄▄ █░█",
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
