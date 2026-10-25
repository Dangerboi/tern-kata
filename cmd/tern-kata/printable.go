package main

import (
	"io"
	"strings"
	"unicode"
)

// printable is s with every control character but newline and tab shown as U+FFFD. Subcommands print text kata answered
// (titles, attachment names, a daemon's error message and hint), which a remote daemon or another user wrote; an escape
// sequence in it must not reach the terminal, where it could move the cursor, rewrite earlier lines or set the window title.
func printable(s string) string {
	if !strings.ContainsFunc(s, unsafeRune) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if unsafeRune(r) {
			return unicode.ReplacementChar
		}
		return r
	}, s)
}

func unsafeRune(r rune) bool { return r != '\n' && r != '\t' && unicode.IsControl(r) }

// printableWriter writes through printable. Every subcommand line is one Write, so a character is never split across two.
type printableWriter struct{ w io.Writer }

func (p printableWriter) Write(b []byte) (int, error) {
	if _, err := io.WriteString(p.w, printable(string(b))); err != nil {
		return 0, err
	}
	return len(b), nil
}
