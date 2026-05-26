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

// Package cli holds CLI output helpers for themed terminal rendering.
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Checkmark is the Unicode check character used in step output.
const Checkmark = "✓"

// Theme is a palette covering agentpack's CLI surface.
type Theme struct {
	Name      string
	Mute      lipgloss.Style
	Accent    lipgloss.Style
	OK        lipgloss.Style
	Err       lipgloss.Style
	Info      lipgloss.Style
	Tag       lipgloss.Style
	BannerTop lipgloss.Style
	BannerBot lipgloss.Style
}

func fg(c string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(c))
}

var faint = lipgloss.NewStyle().Faint(true)

// ThemeDark uses magenta (#c678dd) as the primary accent with
// supporting roles from the same palette. Designed for dark backgrounds.
var ThemeDark = Theme{
	Name:      "dark",
	Mute:      faint,
	Accent:    fg("#c678dd"),
	OK:        fg("#50fa7b"),
	Err:       fg("#ff6ec7"),
	Info:      fg("#00d4ff"),
	Tag:       fg("#ffb86c"),
	BannerTop: faint,
	BannerBot: fg("#c678dd"),
}

// ThemeLight uses the same hues but darker/more saturated for contrast
// against light terminal backgrounds.
var ThemeLight = Theme{
	Name:      "light",
	Mute:      fg("#888888"),
	Accent:    fg("#9b59b6"),
	OK:        fg("#27ae60"),
	Err:       fg("#e74c3c"),
	Info:      fg("#2980b9"),
	Tag:       fg("#d35400"),
	BannerTop: fg("#888888"),
	BannerBot: fg("#9b59b6"),
}

var active = detectTheme()

func detectTheme() *Theme {
	if !termenv.HasDarkBackground() {
		return &ThemeLight
	}

	return &ThemeDark
}

func rendererFor(w io.Writer) *lipgloss.Renderer {
	if f, ok := w.(*os.File); ok {
		return lipgloss.NewRenderer(f)
	}
	return lipgloss.DefaultRenderer()
}

func render(w io.Writer, st lipgloss.Style, s string) string {
	return st.Renderer(rendererFor(w)).Render(s)
}

// Banner returns the AGENTPACK block-letter logo.
func Banner(w io.Writer) string {
	const top = "█▀█ █▀▀ █▀▀ █▄░█ ▀█▀ █▀█ █▀█ █▀▀ █▄▀"
	const bot = "█▀█ █▄█ ██▄ █░▀█ ░█░ █▀▀ █▀█ █▄▄ █░█"
	return render(w, active.BannerTop, top) + "\n" +
		render(w, active.BannerBot, bot) + "\n"
}

// Print writes a line to w.
func Print(w io.Writer, s string) {
	_, _ = fmt.Fprintln(w, s)
}

// Printf writes a formatted string to w.
func Printf(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...)
}

// Mute returns s rendered as secondary text.
func Mute(w io.Writer, s string) string { return render(w, active.Mute, s) }

// Accent returns s in the brand accent color.
func Accent(w io.Writer, s string) string { return render(w, active.Accent, s) }

// OK returns s in the success color.
func OK(w io.Writer, s string) string { return render(w, active.OK, s) }

// Err returns s in the error color.
func Err(w io.Writer, s string) string { return render(w, active.Err, s) }

// Info returns s in the info color (cyan, for dates and versions).
func Info(w io.Writer, s string) string { return render(w, active.Info, s) }

// Tag returns s in the tag color (warm orange, for categories).
func Tag(w io.Writer, s string) string { return render(w, active.Tag, s) }

// Pad right-pads s with spaces to width w.
// If s is already w or more characters, s is returned unchanged.
func Pad(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}

// HumanSize formats a byte count for display.
func HumanSize(bytes int64) string {
	const kb = 1024
	if bytes < kb {
		return fmt.Sprintf("%d B", bytes)
	}
	return fmt.Sprintf("%d KB", bytes/kb)
}

// Plural returns singular when n == 1, otherwise pluralForm.
func Plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}

// ShortSHA returns the first 7 characters of sha, or the full string if shorter.
func ShortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	return sha
}

// SourceBaseName extracts a short display name from a source path or URL.
// It strips ref fragments (#branch or #sha), trailing .git and trailing slashes.
func SourceBaseName(source string) string {
	s := source

	// Strip ref fragment (#branch or #sha).
	if idx := strings.LastIndex(s, "#"); idx >= 0 {
		s = s[:idx]
	}

	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimSuffix(s, "/")

	if idx := strings.LastIndex(s, "/"); idx >= 0 {
		return s[idx+1:]
	}

	return s
}

// Header writes the standard command header: "agentpack: action name\n\n".
func Header(w io.Writer, action, name string) {
	Printf(w, "%s %s\n\n", Mute(w, "agentpack: "+action), Accent(w, name))
}

// StepLine writes a single step result line: "  ✓ name detail\n".
func StepLine(w io.Writer, name, detail string) {
	Printf(w, "  %s %s %s\n", OK(w, Checkmark), Mute(w, name), Mute(w, detail))
}

// TreeRow writes a tree-formatted row for a list of items.
// isLast controls whether to use └─ (last item) or ├─ (non-last).
// name is printed accented and padded to nameWidth; detail is muted.
func TreeRow(w io.Writer, isLast bool, name string, nameWidth int, detail string) {
	prefix := "  ├─"
	if isLast {
		prefix = "  └─"
	}
	Printf(
		w, "%s %s  %s\n",
		Mute(w, prefix),
		Accent(w, Pad(name, nameWidth)),
		Mute(w, detail),
	)
}

// Field writes a label/value pair: "Label: value\n".
func Field(w io.Writer, label, value string) {
	Printf(w, "%s %s\n", Mute(w, label+":"), value)
}

// FieldAccent writes a label/value pair with the value in accent color.
func FieldAccent(w io.Writer, label, value string) {
	Printf(w, "%s %s\n", Mute(w, label+":"), Accent(w, value))
}

// FieldInfo writes a label/value pair with the value in info color.
func FieldInfo(w io.Writer, label, value string) {
	Printf(w, "%s %s\n", Mute(w, label+":"), Info(w, value))
}

// FieldMuted writes a label/value pair where both label and value are muted.
func FieldMuted(w io.Writer, label, value string) {
	Printf(w, "%s %s\n", Mute(w, label+":"), Mute(w, value))
}

// TableColumn holds the display data for a single column in a Table.
type TableColumn struct {
	Header string
	Values []string
	Accent bool
	Info   bool
	Muted  bool
	Tag    bool
}

// Table renders an aligned table with muted headers and themed rows.
// cols defines the columns; the last column's header is printed without trailing padding.
func Table(w io.Writer, cols []TableColumn) {
	const pad = 2

	// Compute column widths from header and all values.
	widths := make([]int, len(cols))
	for i, col := range cols {
		widths[i] = len(col.Header)
		for _, v := range col.Values {
			if len(v) > widths[i] {
				widths[i] = len(v)
			}
		}
	}

	// Print header row.
	var hdr strings.Builder
	for i, col := range cols {
		if i < len(cols)-1 {
			hdr.WriteString(Pad(col.Header, widths[i]+pad))
		} else {
			hdr.WriteString(col.Header)
		}
	}
	Printf(w, "%s\n", Mute(w, hdr.String()))

	// Print data rows.
	rows := 0
	if len(cols) > 0 {
		rows = len(cols[0].Values)
	}
	for r := range rows {
		var line strings.Builder
		for i, col := range cols {
			val := ""
			if r < len(col.Values) {
				val = col.Values[r]
			}
			cell := val
			if i < len(cols)-1 {
				cell = Pad(val, widths[i]+pad)
			}
			var rendered string
			switch {
			case col.Accent:
				rendered = Accent(w, cell)
			case col.Info:
				rendered = Info(w, cell)
			case col.Tag:
				rendered = Tag(w, cell)
			case col.Muted:
				rendered = Mute(w, cell)
			default:
				rendered = cell
			}
			line.WriteString(rendered)
		}
		Printf(w, "%s\n", line.String())
	}
}
