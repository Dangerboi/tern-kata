package main

import (
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

// printable is s with every control character but newline and tab, and every byte that is not valid UTF-8, shown as U+FFFD.
// Subcommands print text kata answered (titles, attachment names, a daemon's error message and hint), which a remote daemon
// or another user wrote; an escape sequence in it must not reach the terminal, where it could move the cursor, rewrite
// earlier lines or set the window title.
func printable(s string) string {
	if utf8.ValidString(s) && !strings.ContainsFunc(s, unsafeRune) {
		return s
	}
	// A byte that is not valid UTF-8 (0x9B is the 8-bit CSI on a legacy terminal) is replaced before the controls are mapped.
	return strings.Map(func(r rune) rune {
		if unsafeRune(r) {
			return unicode.ReplacementChar
		}
		return r
	}, strings.ToValidUTF8(s, string(unicode.ReplacementChar)))
}

func unsafeRune(r rune) bool { return r != '\n' && r != '\t' && unicode.IsControl(r) }

// printableWriter is the one writer every subcommand prints through: it writes through printable, and a failed write is a
// subcommandError (exit 1), not the Pane's start-failure status. Every subcommand line is one Write, so a character is never
// split across two.
type printableWriter struct{ w io.Writer }

func (p printableWriter) Write(b []byte) (int, error) {
	if _, err := io.WriteString(p.w, printable(string(b))); err != nil {
		return 0, subcommandError{err}
	}
	return len(b), nil
}
