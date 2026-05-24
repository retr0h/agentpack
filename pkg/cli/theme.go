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
)

// Theme is a palette covering agentpack's CLI surface.
type Theme struct {
	Name      string
	Mute      lipgloss.Style
	Accent    lipgloss.Style
	OK        lipgloss.Style
	Err       lipgloss.Style
	Info      lipgloss.Style
	BannerTop lipgloss.Style
	BannerBot lipgloss.Style
}

func fg(c string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(c))
}

var faint = lipgloss.NewStyle().Faint(true)

// ThemeClaude uses the Claude Code brand palette — the warm terracotta
// orange (#cc7c5e) sampled from code.claude.com as the primary accent.
var ThemeClaude = Theme{
	Name:      "claude",
	Mute:      faint,
	Accent:    fg("#cc7c5e"), // Claude Code orange (R:204 G:124 B:94)
	OK:        fg("#50fa7b"),
	Err:       fg("#ff6ec7"),
	Info:      fg("#00d4ff"), // cyan, for dates/versions
	BannerTop: faint,
	BannerBot: fg("#cc7c5e"), // Claude Code orange
}

var active = &ThemeClaude

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
